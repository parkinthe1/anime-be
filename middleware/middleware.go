// Package middleware: ตรวจสิทธิ์ (login / admin), CORS, กัน panic, และ log
package middleware

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"aniwave-be/auth"
	"aniwave-be/models"
	"aniwave-be/response"
)

type contextKey string

const memberKey contextKey = "member"

// Authenticator ตรวจ token แล้วไปอ่านสมาชิกจาก DB ทุกครั้ง
// ทำให้สมาชิกที่ถูกลบหรือถูกเปลี่ยนบทบาทมีผลทันที และ FE แก้ localStorage เป็น admin ก็ผ่านไม่ได้
type Authenticator struct {
	DB     *sql.DB
	Tokens *auth.TokenService
}

// RequireLogin: ไม่มี/ผิด/หมดอายุ -> 401
func (a *Authenticator) RequireLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || strings.TrimSpace(token) == "" {
			response.Error(w, http.StatusUnauthorized, "กรุณาเข้าสู่ระบบ")
			return
		}
		id, err := a.Tokens.Parse(strings.TrimSpace(token))
		if err != nil {
			response.Error(w, http.StatusUnauthorized, "เซสชันหมดอายุ กรุณาเข้าสู่ระบบใหม่")
			return
		}

		var m models.Member
		err = a.DB.QueryRowContext(r.Context(),
			`SELECT id, email, display_name, role, created_at FROM members WHERE id = $1`, id,
		).Scan(&m.ID, &m.Email, &m.DisplayName, &m.Role, &m.CreatedAt)
		if errors.Is(err, sql.ErrNoRows) {
			response.Error(w, http.StatusUnauthorized, "ไม่พบบัญชีนี้ กรุณาเข้าสู่ระบบใหม่")
			return
		}
		if err != nil {
			log.Printf("auth lookup: %v", err)
			response.Error(w, http.StatusInternalServerError, "เซิร์ฟเวอร์ผิดพลาด ลองใหม่อีกครั้ง")
			return
		}

		ctx := context.WithValue(r.Context(), memberKey, m)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAdmin: ต้อง login ก่อน (401) แล้วต้องเป็น admin (403)
func (a *Authenticator) RequireAdmin(next http.Handler) http.Handler {
	return a.RequireLogin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if CurrentMember(r).Role != models.RoleAdmin {
			response.Error(w, http.StatusForbidden, "สำหรับ admin เท่านั้น")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

// CurrentMember คืนสมาชิกที่ login อยู่ (ใช้ได้เฉพาะ handler ที่ครอบด้วย RequireLogin / RequireAdmin)
func CurrentMember(r *http.Request) models.Member {
	m, _ := r.Context().Value(memberKey).(models.Member)
	return m
}

type originPattern struct{ scheme, suffix string }

// CORS อนุญาตเฉพาะ origin ในรายการ รองรับรูปแบบ https://*.vercel.app (สำหรับ preview deploy)
func CORS(allowed []string) func(http.Handler) http.Handler {
	exact := map[string]bool{}
	var patterns []originPattern
	for _, o := range allowed {
		o = strings.TrimRight(strings.TrimSpace(o), "/")
		if scheme, suffix, ok := strings.Cut(o, "://*"); ok {
			patterns = append(patterns, originPattern{scheme: scheme + "://", suffix: suffix})
			continue
		}
		exact[o] = true
	}
	allowedOrigin := func(origin string) bool {
		if exact[origin] {
			return true
		}
		for _, p := range patterns {
			if strings.HasPrefix(origin, p.scheme) && strings.HasSuffix(origin, p.suffix) {
				return true
			}
		}
		return false
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Vary", "Origin")
			if origin := r.Header.Get("Origin"); origin != "" && allowedOrigin(origin) {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				h.Set("Access-Control-Max-Age", "600")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Recover เปลี่ยน panic เป็น 500 แบบ JSON แทนที่จะตัด connection
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic: %v", rec)
				response.Error(w, http.StatusInternalServerError, "เซิร์ฟเวอร์ผิดพลาด ลองใหม่อีกครั้ง")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})
}

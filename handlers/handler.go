// Package handlers: ตัวจัดการแต่ละ endpoint ของ API (แยกไฟล์ตามกลุ่ม: auth / me / anime / review / wishlist / admin)
package handlers

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"aniwave-be/auth"
	"aniwave-be/models"
	"aniwave-be/response"

	"github.com/jackc/pgx/v5/pgconn"
)

type Handler struct {
	DB     *sql.DB
	Tokens *auth.TokenService
}

const (
	minPasswordLen = 4  // ตรงกับ docs/api-contract.md ของ FE
	maxPasswordLen = 72 // ขีดจำกัดของ bcrypt
	minNameLen     = 2
	minReviewLen   = 10
	maxReviewLen   = 2000
)

type rowScanner interface {
	Scan(dest ...any) error
}

const memberColumns = "id, email, display_name, role, created_at"

func scanMember(row rowScanner) (models.Member, error) {
	var m models.Member
	err := row.Scan(&m.ID, &m.Email, &m.DisplayName, &m.Role, &m.CreatedAt)
	return m, err
}

const animeSelect = "a.id, a.title, a.title_th, a.year, a.genre, a.rating::float8, a.episodes, a.status, a.poster, a.synopsis"

func scanAnime(row rowScanner) (models.Anime, error) {
	var a models.Anime
	err := row.Scan(&a.ID, &a.Title, &a.TitleTh, &a.Year, &a.Genre, &a.Rating, &a.Episodes, &a.Status, &a.Poster, &a.Synopsis)
	return a, err
}

// parseID อ่าน {id} จาก URL ถ้าไม่ใช่เลขบวกถือว่าไม่พบ
func parseID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

// isPgCode ตรวจรหัส error ของ PostgreSQL: 23505 = ซ้ำ (unique), 23503 = อ้างอิงไม่มี (foreign key)
func isPgCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

func serverError(w http.ResponseWriter, err error) {
	log.Printf("error: %v", err)
	response.Error(w, http.StatusInternalServerError, "เซิร์ฟเวอร์ผิดพลาด ลองใหม่อีกครั้ง")
}

func notFoundAnime(w http.ResponseWriter) {
	response.Error(w, http.StatusNotFound, "ไม่พบอนิเมะเรื่องนี้")
}

func normalizeEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// validatePassword คืนข้อความ error ถ้าไม่ผ่าน (ว่าง = ผ่าน)
func validatePassword(p string) string {
	if len([]rune(p)) < minPasswordLen {
		return "รหัสผ่านต้องยาวอย่างน้อย 4 ตัวอักษร"
	}
	if len(p) > maxPasswordLen {
		return "รหัสผ่านยาวเกินไป (ไม่เกิน 72 ไบต์)"
	}
	return ""
}

func blankToNil(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}

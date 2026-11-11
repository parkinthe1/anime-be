package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"aniwave-be/auth"
	"aniwave-be/models"
	"aniwave-be/response"
)

// POST /api/auth/register -> 201 สมาชิกใหม่
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"displayName"`
	}
	if !response.Decode(w, r, &in) {
		return
	}

	email := normalizeEmail(in.Email)
	name := strings.TrimSpace(in.DisplayName)
	if email == "" || name == "" || in.Password == "" {
		response.Error(w, http.StatusBadRequest, "กรอกข้อมูลไม่ครบ")
		return
	}
	if !emailPattern.MatchString(email) {
		response.Error(w, http.StatusBadRequest, "รูปแบบอีเมลไม่ถูกต้อง")
		return
	}
	if utf8.RuneCountInString(name) < minNameLen {
		response.Error(w, http.StatusBadRequest, "ชื่อที่แสดงต้องยาวอย่างน้อย 2 ตัวอักษร")
		return
	}
	if msg := validatePassword(in.Password); msg != "" {
		response.Error(w, http.StatusBadRequest, msg)
		return
	}

	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		serverError(w, err)
		return
	}

	m, err := scanMember(h.DB.QueryRowContext(r.Context(),
		`INSERT INTO members (email, password_hash, display_name) VALUES ($1, $2, $3)
		 RETURNING `+memberColumns, email, hash, name))
	if isPgCode(err, "23505") {
		response.Error(w, http.StatusConflict, "อีเมลนี้ถูกใช้แล้ว")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, m)
}

// POST /api/auth/login -> 200 { token, member }
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !response.Decode(w, r, &in) {
		return
	}

	var (
		m    models.Member
		hash string
	)
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT `+memberColumns+`, password_hash FROM members WHERE lower(email) = $1`,
		normalizeEmail(in.Email),
	).Scan(&m.ID, &m.Email, &m.DisplayName, &m.Role, &m.CreatedAt, &hash)

	// ไม่มีอีเมลนี้ กับ รหัสผ่านผิด ตอบข้อความเดียวกัน เพื่อไม่บอกคนนอกว่าอีเมลไหนมีในระบบ
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !auth.CheckPassword(hash, in.Password)) {
		response.Error(w, http.StatusUnauthorized, "อีเมลหรือรหัสผ่านไม่ถูกต้อง")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}

	token, err := h.Tokens.Issue(m.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"token": token, "member": m})
}

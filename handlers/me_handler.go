package handlers

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"musicken-be/auth"
	"musicken-be/middleware"
	"musicken-be/response"
)

// GET /api/me
func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]any{"member": middleware.CurrentMember(r)})
}

// PUT /api/me  { displayName }
func (h *Handler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DisplayName string `json:"displayName"`
	}
	if !response.Decode(w, r, &in) {
		return
	}
	name := strings.TrimSpace(in.DisplayName)
	if utf8.RuneCountInString(name) < minNameLen {
		response.Error(w, http.StatusBadRequest, "ชื่อที่แสดงต้องยาวอย่างน้อย 2 ตัวอักษร")
		return
	}

	me := middleware.CurrentMember(r)
	m, err := scanMember(h.DB.QueryRowContext(r.Context(),
		`UPDATE members SET display_name = $1, updated_at = now() WHERE id = $2 RETURNING `+memberColumns,
		name, me.ID))
	if err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"member": m})
}

// PUT /api/me/password  { currentPassword, newPassword }
func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if !response.Decode(w, r, &in) {
		return
	}

	me := middleware.CurrentMember(r)
	var hash string
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT password_hash FROM members WHERE id = $1`, me.ID).Scan(&hash); err != nil {
		serverError(w, err)
		return
	}
	if !auth.CheckPassword(hash, in.CurrentPassword) {
		response.Error(w, http.StatusBadRequest, "รหัสผ่านปัจจุบันไม่ถูกต้อง")
		return
	}
	if msg := validatePassword(in.NewPassword); msg != "" {
		response.Error(w, http.StatusBadRequest, "รหัสผ่านใหม่"+strings.TrimPrefix(msg, "รหัสผ่าน"))
		return
	}

	newHash, err := auth.HashPassword(in.NewPassword)
	if err != nil {
		serverError(w, err)
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE members SET password_hash = $1, updated_at = now() WHERE id = $2`, newHash, me.ID); err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

package handlers

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"aniwave-be/response"
)

// ข้อมูลอนิเมะ (ชื่อ โปสเตอร์ เรื่องย่อ) FE ดึงจาก TENRAI เอง ตารางนี้เก็บแค่ที่ admin "แก้ทับ" ข้อมูลของ TENRAI
// anime_id = mal_id ของ TENRAI แต่ละช่องเป็น null = ไม่ได้แก้ ให้ FE ใช้ของ TENRAI ช่องนั้น
type animeOverride struct {
	ID       int64   `json:"id"`
	TitleTh  *string `json:"titleTh"`
	Genre    *string `json:"genre"`
	Poster   *string `json:"poster"`
	Synopsis *string `json:"synopsis"`
}

const (
	maxOverrideTitleTh  = 255  // ตรงกับ VARCHAR(255) ใน schema.sql
	maxOverrideGenre    = 50   // ตรงกับ VARCHAR(50)
	maxOverridePoster   = 2000 // ลิงก์รูป
	maxOverrideSynopsis = 5000
)

// validateOverride คืนข้อความ error ถ้าไม่ผ่าน (ว่าง = ผ่าน)
func validateOverride(o animeOverride) string {
	if o.TitleTh != nil && utf8.RuneCountInString(*o.TitleTh) > maxOverrideTitleTh {
		return "ชื่อไทยยาวเกินไป (ไม่เกิน 255 ตัวอักษร)"
	}
	if o.Genre != nil && utf8.RuneCountInString(*o.Genre) > maxOverrideGenre {
		return "แนวยาวเกินไป (ไม่เกิน 50 ตัวอักษร)"
	}
	if o.Synopsis != nil && utf8.RuneCountInString(*o.Synopsis) > maxOverrideSynopsis {
		return "เรื่องย่อยาวเกินไป (ไม่เกิน 5000 ตัวอักษร)"
	}
	if o.Poster != nil {
		if utf8.RuneCountInString(*o.Poster) > maxOverridePoster {
			return "ลิงก์โปสเตอร์ยาวเกินไป"
		}
		p := strings.ToLower(*o.Poster)
		if !strings.HasPrefix(p, "http://") && !strings.HasPrefix(p, "https://") {
			return "ลิงก์โปสเตอร์ต้องขึ้นต้นด้วย http:// หรือ https://"
		}
	}
	return ""
}

// GET /api/anime-overrides -> { items: [{ id, titleTh, genre, poster, synopsis }] }  (guest ดูได้)
func (h *Handler) ListOverrides(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT anime_id, title_th, genre, poster, synopsis FROM anime_overrides ORDER BY anime_id`)
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()

	items := make([]animeOverride, 0) // ไม่ใช้ nil เพื่อให้ JSON เป็น [] ไม่ใช่ null
	for rows.Next() {
		var o animeOverride
		if err := rows.Scan(&o.ID, &o.TitleTh, &o.Genre, &o.Poster, &o.Synopsis); err != nil {
			serverError(w, err)
			return
		}
		items = append(items, o)
	}
	if err := rows.Err(); err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// PUT /api/admin/anime-overrides/{id}  { titleTh, genre, poster, synopsis }  (admin) -> 200 override
// ส่งมาทั้ง 4 ช่องเสมอ ช่องที่ว่าง/null = ใช้ของ TENRAI (ค่าเดิมในตารางถูกเขียนทับทั้งแถว)
func (h *Handler) AdminPutOverride(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		badAnimeID(w)
		return
	}
	var in struct {
		TitleTh  *string `json:"titleTh"`
		Genre    *string `json:"genre"`
		Poster   *string `json:"poster"`
		Synopsis *string `json:"synopsis"`
	}
	if !response.Decode(w, r, &in) {
		return
	}

	o := animeOverride{
		ID:       id,
		TitleTh:  blankToNil(in.TitleTh),
		Genre:    blankToNil(in.Genre),
		Poster:   blankToNil(in.Poster),
		Synopsis: blankToNil(in.Synopsis),
	}
	if o.TitleTh == nil && o.Genre == nil && o.Poster == nil && o.Synopsis == nil {
		response.Error(w, http.StatusBadRequest, "ไม่มีข้อมูลที่จะแก้ไข ถ้าจะกลับไปใช้ข้อมูลของ TENRAI ให้ลบแทน")
		return
	}
	if msg := validateOverride(o); msg != "" {
		response.Error(w, http.StatusBadRequest, msg)
		return
	}

	_, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO anime_overrides (anime_id, title_th, genre, poster, synopsis) VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (anime_id) DO UPDATE
		 SET title_th = EXCLUDED.title_th, genre = EXCLUDED.genre, poster = EXCLUDED.poster,
		     synopsis = EXCLUDED.synopsis, updated_at = now()`,
		o.ID, o.TitleTh, o.Genre, o.Poster, o.Synopsis)
	if err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, o)
}

// DELETE /api/admin/anime-overrides/{id}  (admin) -> 204  กลับไปใช้ข้อมูลของ TENRAI ทั้งหมด (ลบซ้ำได้ ไม่ error)
func (h *Handler) AdminDeleteOverride(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		badAnimeID(w)
		return
	}
	if _, err := h.DB.ExecContext(r.Context(), `DELETE FROM anime_overrides WHERE anime_id = $1`, id); err != nil {
		serverError(w, err)
		return
	}
	response.NoContent(w)
}

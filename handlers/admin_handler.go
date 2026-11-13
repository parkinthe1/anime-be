package handlers

import (
	"database/sql"
	"errors"
	"net/http"

	"musicken-be/middleware"
	"musicken-be/models"
	"musicken-be/response"
)

// ทุก endpoint ในไฟล์นี้ (และ AdminCreate/Update/Delete ของเพลงกับศิลปิน) ถูกครอบด้วย RequireAdmin ใน router
// ไม่ login = 401, เป็น member = 403

type topSongItem struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	ArtistName string `json:"artistName"`
	Plays      int    `json:"plays"`
}

type statsResponse struct {
	Members       int             `json:"members"`
	Admins        int             `json:"admins"`
	Songs         int             `json:"songs"`
	Artists       int             `json:"artists"`
	Playlists     int             `json:"playlists"`
	Plays         int             `json:"plays"`
	TopSongs      []topSongItem   `json:"topSongs"`
	NewestMembers []models.Member `json:"newestMembers"`
}

// GET /api/admin/stats
func (h *Handler) AdminStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s := statsResponse{TopSongs: make([]topSongItem, 0), NewestMembers: make([]models.Member, 0)}

	err := h.DB.QueryRowContext(ctx, `
		SELECT (SELECT COUNT(*) FROM members),
		       (SELECT COUNT(*) FROM members WHERE role = 'admin'),
		       (SELECT COUNT(*) FROM songs),
		       (SELECT COUNT(*) FROM artists),
		       (SELECT COUNT(*) FROM playlists),
		       (SELECT COUNT(*) FROM plays)`,
	).Scan(&s.Members, &s.Admins, &s.Songs, &s.Artists, &s.Playlists, &s.Plays)
	if err != nil {
		serverError(w, err)
		return
	}

	top, err := h.DB.QueryContext(ctx, `
		SELECT s.id, s.title, ar.name, COUNT(p.id) AS plays
		FROM plays p JOIN songs s ON s.id = p.song_id JOIN artists ar ON ar.id = s.artist_id
		GROUP BY s.id, s.title, ar.name
		ORDER BY plays DESC, s.id
		LIMIT 5`)
	if err != nil {
		serverError(w, err)
		return
	}
	defer top.Close()
	for top.Next() {
		var t topSongItem
		if err := top.Scan(&t.ID, &t.Title, &t.ArtistName, &t.Plays); err != nil {
			serverError(w, err)
			return
		}
		s.TopSongs = append(s.TopSongs, t)
	}
	if err := top.Err(); err != nil {
		serverError(w, err)
		return
	}

	newest, err := h.DB.QueryContext(ctx,
		`SELECT `+memberColumns+` FROM members ORDER BY created_at DESC, id DESC LIMIT 5`)
	if err != nil {
		serverError(w, err)
		return
	}
	defer newest.Close()
	for newest.Next() {
		m, err := scanMember(newest)
		if err != nil {
			serverError(w, err)
			return
		}
		s.NewestMembers = append(s.NewestMembers, m)
	}
	if err := newest.Err(); err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, s)
}

// GET /api/admin/members -> { items }
func (h *Handler) AdminMembers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(), `SELECT `+memberColumns+` FROM members ORDER BY id`)
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()

	items := make([]models.Member, 0)
	for rows.Next() {
		m, err := scanMember(rows)
		if err != nil {
			serverError(w, err)
			return
		}
		items = append(items, m)
	}
	if err := rows.Err(); err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// PUT /api/admin/members/{id}/role  { role } -> 200 member
func (h *Handler) AdminSetRole(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		notFound(w, "สมาชิก")
		return
	}
	var in struct {
		Role string `json:"role"`
	}
	if !response.Decode(w, r, &in) {
		return
	}
	if in.Role != models.RoleMember && in.Role != models.RoleAdmin {
		response.Error(w, http.StatusBadRequest, "role ต้องเป็น member หรือ admin")
		return
	}
	if id == middleware.CurrentMember(r).ID {
		response.Error(w, http.StatusBadRequest, "เปลี่ยนบทบาทของตัวเองไม่ได้ ให้ admin คนอื่นทำแทน")
		return
	}

	m, err := scanMember(h.DB.QueryRowContext(r.Context(),
		`UPDATE members SET role = $1, updated_at = now() WHERE id = $2 RETURNING `+memberColumns, in.Role, id))
	if errors.Is(err, sql.ErrNoRows) {
		notFound(w, "สมาชิก")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, m)
}

// GET /api/admin/playlists -> { items } ทุกเพลย์ลิสต์ รวมที่เป็นส่วนตัว (ไว้ตรวจดูแลเนื้อหา)
func (h *Handler) AdminListPlaylists(w http.ResponseWriter, r *http.Request) {
	items, err := h.listPlaylists(r.Context(), `ORDER BY p.id DESC LIMIT 200`)
	if err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// DELETE /api/admin/playlists/{id} -> 204 ลบเพลย์ลิสต์ที่ไม่เหมาะสม (ของใครก็ได้)
func (h *Handler) AdminDeletePlaylist(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		notFound(w, "เพลย์ลิสต์นี้")
		return
	}
	res, err := h.DB.ExecContext(r.Context(), `DELETE FROM playlists WHERE id = $1`, id)
	if err != nil {
		serverError(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		notFound(w, "เพลย์ลิสต์นี้")
		return
	}
	response.NoContent(w)
}

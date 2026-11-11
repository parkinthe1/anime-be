package handlers

import (
	"net/http"

	"aniwave-be/middleware"
	"aniwave-be/models"
	"aniwave-be/response"
)

// GET /api/me/wishlist -> { items: [Anime] }
func (h *Handler) ListWishlist(w http.ResponseWriter, r *http.Request) {
	me := middleware.CurrentMember(r)
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT `+animeSelect+`
		 FROM wishlist_items wi JOIN anime a ON a.id = wi.anime_id
		 WHERE wi.member_id = $1
		 ORDER BY wi.added_at DESC`, me.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()

	items := make([]models.Anime, 0)
	for rows.Next() {
		a, err := scanAnime(rows)
		if err != nil {
			serverError(w, err)
			return
		}
		items = append(items, a)
	}
	if err := rows.Err(); err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// PUT /api/me/wishlist/{id} (เพิ่มซ้ำได้ ไม่ error)
func (h *Handler) AddWishlist(w http.ResponseWriter, r *http.Request) {
	animeID, ok := parseID(r)
	if !ok {
		notFoundAnime(w)
		return
	}
	me := middleware.CurrentMember(r)
	_, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO wishlist_items (member_id, anime_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		me.ID, animeID)
	if isPgCode(err, "23503") {
		notFoundAnime(w)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]int64{"animeId": animeID})
}

// DELETE /api/me/wishlist/{id} -> 204
func (h *Handler) RemoveWishlist(w http.ResponseWriter, r *http.Request) {
	animeID, ok := parseID(r)
	if !ok {
		response.NoContent(w)
		return
	}
	me := middleware.CurrentMember(r)
	if _, err := h.DB.ExecContext(r.Context(),
		`DELETE FROM wishlist_items WHERE member_id = $1 AND anime_id = $2`, me.ID, animeID); err != nil {
		serverError(w, err)
		return
	}
	response.NoContent(w)
}

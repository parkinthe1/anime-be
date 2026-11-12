package handlers

import (
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"aniwave-be/middleware"
	"aniwave-be/response"
)

type reviewAuthor struct {
	ID          int64  `json:"id"`
	DisplayName string `json:"displayName"`
}

type reviewItem struct {
	ID        int64        `json:"id"`
	Text      string       `json:"text"`
	CreatedAt time.Time    `json:"createdAt"`
	Score     *int         `json:"score"` // คะแนนที่คนเขียนรีวิวให้เรื่องนี้ (null ถ้าไม่ได้โหวต)
	Member    reviewAuthor `json:"member"`
}

// GET /api/anime/{id}/reviews -> { items }  (guest ดูได้)
func (h *Handler) ListReviews(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		notFoundAnime(w)
		return
	}

	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT r.id, r.text, r.created_at, v.score, m.id, m.display_name
		 FROM reviews r
		 JOIN members m ON m.id = r.member_id
		 LEFT JOIN votes v ON v.member_id = r.member_id AND v.anime_id = r.anime_id
		 WHERE r.anime_id = $1
		 ORDER BY r.created_at DESC, r.id DESC
		 LIMIT 100`, id)
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()

	items := make([]reviewItem, 0)
	for rows.Next() {
		var it reviewItem
		if err := rows.Scan(&it.ID, &it.Text, &it.CreatedAt, &it.Score, &it.Member.ID, &it.Member.DisplayName); err != nil {
			serverError(w, err)
			return
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// POST /api/anime/{id}/reviews  { text }  (member) -> 201 { id, text, createdAt }
func (h *Handler) CreateReview(w http.ResponseWriter, r *http.Request) {
	animeID, ok := parseID(r)
	if !ok {
		badAnimeID(w)
		return
	}
	var in struct {
		Text string `json:"text"`
	}
	if !response.Decode(w, r, &in) {
		return
	}
	text := strings.TrimSpace(in.Text)
	if n := utf8.RuneCountInString(text); n < minReviewLen {
		response.Error(w, http.StatusBadRequest, "รีวิวสั้นเกินไป (อย่างน้อย 10 ตัวอักษร)")
		return
	} else if n > maxReviewLen {
		response.Error(w, http.StatusBadRequest, "รีวิวยาวเกินไป (ไม่เกิน 2000 ตัวอักษร)")
		return
	}

	// member_id มาจาก token เท่านั้น ไม่รับจาก body/URL
	me := middleware.CurrentMember(r)
	var out struct {
		ID        int64     `json:"id"`
		Text      string    `json:"text"`
		CreatedAt time.Time `json:"createdAt"`
	}
	err := h.DB.QueryRowContext(r.Context(),
		`INSERT INTO reviews (member_id, anime_id, text) VALUES ($1, $2, $3) RETURNING id, text, created_at`,
		me.ID, animeID, text,
	).Scan(&out.ID, &out.Text, &out.CreatedAt)
	if err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, out)
}

// PUT /api/anime/{id}/vote  { score 1-10 }  (member) -> { score, voteCount, voteAverage }
func (h *Handler) PutVote(w http.ResponseWriter, r *http.Request) {
	animeID, ok := parseID(r)
	if !ok {
		badAnimeID(w)
		return
	}
	var in struct {
		Score int `json:"score"`
	}
	if !response.Decode(w, r, &in) {
		return
	}
	if in.Score < 1 || in.Score > 10 {
		response.Error(w, http.StatusBadRequest, "คะแนนต้องเป็นเลข 1 ถึง 10")
		return
	}

	me := middleware.CurrentMember(r)
	_, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO votes (member_id, anime_id, score) VALUES ($1, $2, $3)
		 ON CONFLICT (member_id, anime_id) DO UPDATE SET score = EXCLUDED.score, updated_at = now()`,
		me.ID, animeID, in.Score)
	if err != nil {
		serverError(w, err)
		return
	}

	var (
		count int
		avg   *float64
	)
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*), ROUND(AVG(score), 1)::float8 FROM votes WHERE anime_id = $1`, animeID,
	).Scan(&count, &avg); err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"score": in.Score, "voteCount": count, "voteAverage": avg})
}

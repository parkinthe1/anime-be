package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"aniwave-be/models"
	"aniwave-be/response"
)

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// animeDetail = Anime + คะแนนเฉลี่ยจากสมาชิกเว็บเรา (voteAverage เป็น null ถ้ายังไม่มีใครโหวต)
type animeDetail struct {
	models.Anime
	VoteCount   int      `json:"voteCount"`
	VoteAverage *float64 `json:"voteAverage"`
}

// GET /api/anime?q=คำค้น  -> { items: [Anime] }
func (h *Handler) ListAnime(w http.ResponseWriter, r *http.Request) {
	q := likeEscaper.Replace(strings.TrimSpace(r.URL.Query().Get("q")))

	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT `+animeSelect+` FROM anime a
		 WHERE ($1 = '' OR a.title ILIKE '%' || $1 || '%' OR COALESCE(a.title_th, '') ILIKE '%' || $1 || '%')
		 ORDER BY a.id`, q)
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()

	items := make([]models.Anime, 0) // ไม่ใช้ nil เพื่อให้ JSON เป็น [] ไม่ใช่ null (FE เรียก .length / .map)
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

// GET /api/anime/{id}
func (h *Handler) GetAnime(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		notFoundAnime(w)
		return
	}

	var d animeDetail
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT `+animeSelect+`,
		        (SELECT COUNT(*) FROM votes v WHERE v.anime_id = a.id),
		        (SELECT ROUND(AVG(v.score), 1)::float8 FROM votes v WHERE v.anime_id = a.id)
		 FROM anime a WHERE a.id = $1`, id,
	).Scan(&d.ID, &d.Title, &d.TitleTh, &d.Year, &d.Genre, &d.Rating, &d.Episodes, &d.Status, &d.Poster, &d.Synopsis,
		&d.VoteCount, &d.VoteAverage)
	if errors.Is(err, sql.ErrNoRows) {
		notFoundAnime(w)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, d)
}

// GET /api/anime/{id}/stats -> { voteCount, voteAverage }  (guest ดูได้)
// id = mal_id ของ TENRAI ยังไม่มีใครโหวต = voteCount 0 และ voteAverage null
func (h *Handler) AnimeStats(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		badAnimeID(w)
		return
	}

	var out struct {
		VoteCount   int      `json:"voteCount"`
		VoteAverage *float64 `json:"voteAverage"`
	}
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*), ROUND(AVG(score), 1)::float8 FROM votes WHERE anime_id = $1`, id,
	).Scan(&out.VoteCount, &out.VoteAverage); err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, out)
}

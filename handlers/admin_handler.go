package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"aniwave-be/middleware"
	"aniwave-be/models"
	"aniwave-be/response"
)

// ทุก endpoint ในไฟล์นี้ถูกครอบด้วย RequireAdmin ใน router: ไม่ login = 401, เป็น member = 403

// id = mal_id ของ TENRAI ไม่ join ตาราง anime เพราะ id อาจไปตรงกับเรื่องอื่นในรายการของทีม (FE ไปดึงชื่อจาก TENRAI เอง)
type topVotedItem struct {
	ID          int64   `json:"id"`
	VoteAverage float64 `json:"voteAverage"`
	VoteCount   int     `json:"voteCount"`
}

type statsResponse struct {
	Anime         int             `json:"anime"`
	Members       int             `json:"members"`
	Admins        int             `json:"admins"`
	Votes         int             `json:"votes"`
	WishlistItems int             `json:"wishlistItems"`
	TopVoted      []topVotedItem  `json:"topVoted"`
	NewestMembers []models.Member `json:"newestMembers"`
}

// GET /api/admin/stats
func (h *Handler) AdminStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s := statsResponse{TopVoted: make([]topVotedItem, 0), NewestMembers: make([]models.Member, 0)}

	err := h.DB.QueryRowContext(ctx, `
		SELECT (SELECT COUNT(*) FROM anime),
		       (SELECT COUNT(*) FROM members),
		       (SELECT COUNT(*) FROM members WHERE role = 'admin'),
		       (SELECT COUNT(*) FROM votes),
		       (SELECT COUNT(*) FROM wishlist_items)`,
	).Scan(&s.Anime, &s.Members, &s.Admins, &s.Votes, &s.WishlistItems)
	if err != nil {
		serverError(w, err)
		return
	}

	top, err := h.DB.QueryContext(ctx, `
		SELECT v.anime_id, ROUND(AVG(v.score), 1)::float8 AS avg_score, COUNT(*) AS vote_count
		FROM votes v
		GROUP BY v.anime_id
		ORDER BY avg_score DESC, vote_count DESC
		LIMIT 5`)
	if err != nil {
		serverError(w, err)
		return
	}
	defer top.Close()
	for top.Next() {
		var t topVotedItem
		if err := top.Scan(&t.ID, &t.VoteAverage, &t.VoteCount); err != nil {
			serverError(w, err)
			return
		}
		s.TopVoted = append(s.TopVoted, t)
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
		response.Error(w, http.StatusNotFound, "ไม่พบสมาชิก")
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
		response.Error(w, http.StatusNotFound, "ไม่พบสมาชิก")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, m)
}

// ---------- จัดการอนิเมะ ----------

type animeInput struct {
	Title    string   `json:"title"`
	TitleTh  *string  `json:"titleTh"`
	Year     *int     `json:"year"`
	Genre    *string  `json:"genre"`
	Rating   *float64 `json:"rating"`
	Episodes *int     `json:"episodes"`
	Status   string   `json:"status"`
	Poster   *string  `json:"poster"`
	Synopsis string   `json:"synopsis"`
}

// clean ตรวจและจัดรูปข้อมูล คืนข้อความ error ถ้าไม่ผ่าน (ว่าง = ผ่าน)
func (in *animeInput) clean() string {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		return "ต้องใส่ชื่อเรื่อง"
	}
	in.TitleTh = blankToNil(in.TitleTh)
	in.Genre = blankToNil(in.Genre)
	in.Poster = blankToNil(in.Poster)
	in.Synopsis = strings.TrimSpace(in.Synopsis)

	if in.Status == "" {
		in.Status = "finished"
	}
	if !models.ValidStatuses[in.Status] {
		return "status ต้องเป็น airing, finished หรือ upcoming"
	}
	if in.Year != nil && (*in.Year < 1900 || *in.Year > 2100) {
		return "ปีต้องอยู่ระหว่าง 1900 ถึง 2100"
	}
	if in.Episodes != nil && *in.Episodes < 0 {
		return "จำนวนตอนต้องไม่ติดลบ"
	}
	if in.Rating != nil && (*in.Rating < 0 || *in.Rating > 10) {
		return "rating ต้องอยู่ระหว่าง 0 ถึง 10"
	}
	return ""
}

// GET /api/admin/anime/{id}
func (h *Handler) AdminGetAnime(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		notFoundAnime(w)
		return
	}
	a, err := scanAnime(h.DB.QueryRowContext(r.Context(),
		`SELECT `+animeSelect+` FROM anime a WHERE a.id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		notFoundAnime(w)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, a)
}

// POST /api/admin/anime -> 201 Anime
func (h *Handler) AdminCreateAnime(w http.ResponseWriter, r *http.Request) {
	var in animeInput
	if !response.Decode(w, r, &in) {
		return
	}
	if msg := in.clean(); msg != "" {
		response.Error(w, http.StatusBadRequest, msg)
		return
	}

	a, err := scanAnime(h.DB.QueryRowContext(r.Context(),
		`INSERT INTO anime AS a (title, title_th, year, genre, rating, episodes, status, poster, synopsis)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING `+animeSelect,
		in.Title, in.TitleTh, in.Year, in.Genre, in.Rating, in.Episodes, in.Status, in.Poster, in.Synopsis))
	if err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, a)
}

// PUT /api/admin/anime/{id} -> 200 Anime
// rating ที่ไม่ได้ส่งมา (ฟอร์มของ FE ไม่มีช่องนี้) จะคงค่าเดิมไว้ ไม่ถูกลบ
func (h *Handler) AdminUpdateAnime(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		notFoundAnime(w)
		return
	}
	var in animeInput
	if !response.Decode(w, r, &in) {
		return
	}
	if msg := in.clean(); msg != "" {
		response.Error(w, http.StatusBadRequest, msg)
		return
	}

	a, err := scanAnime(h.DB.QueryRowContext(r.Context(),
		`UPDATE anime AS a
		 SET title = $1, title_th = $2, year = $3, genre = $4, rating = COALESCE($5::numeric, a.rating),
		     episodes = $6, status = $7, poster = $8, synopsis = $9, updated_at = now()
		 WHERE a.id = $10
		 RETURNING `+animeSelect,
		in.Title, in.TitleTh, in.Year, in.Genre, in.Rating, in.Episodes, in.Status, in.Poster, in.Synopsis, id))
	if errors.Is(err, sql.ErrNoRows) {
		notFoundAnime(w)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, a)
}

// DELETE /api/admin/anime/{id} -> 204 (รีวิว/คะแนน/wishlist ไม่ถูกลบ เพราะอ้างด้วย id ของ TENRAI ไม่ใช่ id ในตาราง anime)
func (h *Handler) AdminDeleteAnime(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		notFoundAnime(w)
		return
	}
	res, err := h.DB.ExecContext(r.Context(), `DELETE FROM anime WHERE id = $1`, id)
	if err != nil {
		serverError(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		notFoundAnime(w)
		return
	}
	response.NoContent(w)
}

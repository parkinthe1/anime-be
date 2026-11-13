package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"musicken-be/models"
	"musicken-be/response"
)

const artistSelect = `ar.id, ar.name, ar.bio, ar.image_url, (SELECT COUNT(*) FROM songs s WHERE s.artist_id = ar.id)`

func scanArtist(row rowScanner) (models.Artist, error) {
	var a models.Artist
	err := row.Scan(&a.ID, &a.Name, &a.Bio, &a.ImageURL, &a.SongCount)
	return a, err
}

// ---------- guest ----------

// GET /api/artists?q= -> { items: [Artist] }
func (h *Handler) ListArtists(w http.ResponseWriter, r *http.Request) {
	q := likeEscaper.Replace(strings.TrimSpace(r.URL.Query().Get("q")))
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT `+artistSelect+` FROM artists ar
		 WHERE ($1 = '' OR ar.name ILIKE '%' || $1 || '%')
		 ORDER BY ar.name LIMIT 200`, q)
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()

	items := make([]models.Artist, 0)
	for rows.Next() {
		a, err := scanArtist(rows)
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

// GET /api/artists/{id} -> Artist + songs
func (h *Handler) GetArtist(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		notFound(w, "ศิลปินนี้")
		return
	}
	a, err := scanArtist(h.DB.QueryRowContext(r.Context(), `SELECT `+artistSelect+` FROM artists ar WHERE ar.id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		notFound(w, "ศิลปินนี้")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}

	rows, err := h.DB.QueryContext(r.Context(), `SELECT `+songSelect+songFrom+` WHERE s.artist_id = $1 ORDER BY s.year DESC NULLS LAST, s.id`, id)
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()
	songs := make([]models.Song, 0)
	for rows.Next() {
		s, err := scanSong(rows)
		if err != nil {
			serverError(w, err)
			return
		}
		songs = append(songs, s)
	}
	if err := rows.Err(); err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, struct {
		models.Artist
		Songs []models.Song `json:"songs"`
	}{a, songs})
}

// ---------- admin ----------

type artistInput struct {
	Name     string  `json:"name"`
	Bio      *string `json:"bio"`
	ImageURL *string `json:"imageUrl"`
}

func (in *artistInput) clean() string {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 200 {
		return "ชื่อศิลปินต้องยาว 1-200 ตัวอักษร"
	}
	in.Bio = blankToNil(in.Bio)
	in.ImageURL = blankToNil(in.ImageURL)
	if in.Bio != nil && utf8.RuneCountInString(*in.Bio) > 2000 {
		return "ประวัติศิลปินยาวเกิน 2000 ตัวอักษร"
	}
	if !validURL(in.ImageURL) {
		return "ลิงก์รูปต้องขึ้นต้นด้วย http:// หรือ https://"
	}
	return ""
}

// POST /api/admin/artists -> 201 Artist
func (h *Handler) AdminCreateArtist(w http.ResponseWriter, r *http.Request) {
	var in artistInput
	if !response.Decode(w, r, &in) {
		return
	}
	if msg := in.clean(); msg != "" {
		response.Error(w, http.StatusBadRequest, msg)
		return
	}
	a, err := scanArtist(h.DB.QueryRowContext(r.Context(),
		`WITH ins AS (INSERT INTO artists (name, bio, image_url) VALUES ($1, $2, $3) RETURNING id, name, bio, image_url)
		 SELECT id, name, bio, image_url, 0 FROM ins`, in.Name, in.Bio, in.ImageURL))
	if isPgCode(err, "23505") {
		response.Error(w, http.StatusConflict, "มีศิลปินชื่อนี้แล้ว")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, a)
}

// PUT /api/admin/artists/{id} -> 200 Artist
func (h *Handler) AdminUpdateArtist(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		notFound(w, "ศิลปินนี้")
		return
	}
	var in artistInput
	if !response.Decode(w, r, &in) {
		return
	}
	if msg := in.clean(); msg != "" {
		response.Error(w, http.StatusBadRequest, msg)
		return
	}
	res, err := h.DB.ExecContext(r.Context(),
		`UPDATE artists SET name = $1, bio = $2, image_url = $3 WHERE id = $4`, in.Name, in.Bio, in.ImageURL, id)
	if isPgCode(err, "23505") {
		response.Error(w, http.StatusConflict, "มีศิลปินชื่อนี้แล้ว")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		notFound(w, "ศิลปินนี้")
		return
	}
	a, err := scanArtist(h.DB.QueryRowContext(r.Context(), `SELECT `+artistSelect+` FROM artists ar WHERE ar.id = $1`, id))
	if err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, a)
}

// DELETE /api/admin/artists/{id} -> 204 · 409 ถ้ายังมีเพลงของศิลปินนี้อยู่
func (h *Handler) AdminDeleteArtist(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		notFound(w, "ศิลปินนี้")
		return
	}
	res, err := h.DB.ExecContext(r.Context(), `DELETE FROM artists WHERE id = $1`, id)
	if isPgCode(err, "23503") {
		response.Error(w, http.StatusConflict, "ลบไม่ได้ เพราะยังมีเพลงของศิลปินนี้อยู่ ลบหรือย้ายเพลงก่อน")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		notFound(w, "ศิลปินนี้")
		return
	}
	response.NoContent(w)
}

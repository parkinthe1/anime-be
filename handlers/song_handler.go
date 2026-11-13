package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"musicken-be/models"
	"musicken-be/response"
)

// ---------- guest ----------

// GET /api/songs?q=&genre=&limit=&offset= -> { items: [Song], total }
func (h *Handler) ListSongs(w http.ResponseWriter, r *http.Request) {
	q := likeEscaper.Replace(strings.TrimSpace(r.URL.Query().Get("q")))
	genre := strings.TrimSpace(r.URL.Query().Get("genre"))
	limit := intQuery(r, "limit", 24, 1, 100)
	offset := intQuery(r, "offset", 0, 0, 1000000)

	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT `+songSelect+`, COUNT(*) OVER() `+songFrom+`
		 WHERE ($1 = '' OR s.title ILIKE '%' || $1 || '%' OR ar.name ILIKE '%' || $1 || '%')
		   AND ($2 = '' OR s.genre = $2)
		 ORDER BY s.id DESC LIMIT $3 OFFSET $4`, q, genre, limit, offset)
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()

	items := make([]models.Song, 0) // ไม่ใช้ nil เพื่อให้ JSON เป็น [] ไม่ใช่ null (FE เรียก .length / .map)
	total := 0
	for rows.Next() {
		s, err := scanSong(rows, &total)
		if err != nil {
			serverError(w, err)
			return
		}
		items = append(items, s)
	}
	if err := rows.Err(); err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

// GET /api/genres -> { items: ["Pop", "Rock", ...] } ไว้ทำตัวกรองแนวเพลง
func (h *Handler) ListGenres(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT DISTINCT genre FROM songs WHERE genre IS NOT NULL AND genre <> '' ORDER BY genre`)
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()

	items := make([]string, 0)
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			serverError(w, err)
			return
		}
		items = append(items, g)
	}
	if err := rows.Err(); err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) getSong(ctx context.Context, id int64) (models.Song, error) {
	return scanSong(h.DB.QueryRowContext(ctx, `SELECT `+songSelect+songFrom+` WHERE s.id = $1`, id))
}

// GET /api/songs/{id}
func (h *Handler) GetSong(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		notFound(w, "เพลงนี้")
		return
	}
	s, err := h.getSong(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		notFound(w, "เพลงนี้")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, s)
}

// ---------- admin ----------

type songInput struct {
	Title       string  `json:"title"`
	ArtistID    int64   `json:"artistId"`
	ArtistName  string  `json:"artistName"` // ใช้แทน artistId ได้: ถ้ายังไม่มีศิลปินชื่อนี้จะสร้างให้ (สะดวกตอนนำเข้าจาก iTunes)
	Album       *string `json:"album"`
	Genre       *string `json:"genre"`
	Year        *int    `json:"year"`
	DurationSec *int    `json:"durationSec"`
	CoverURL    *string `json:"coverUrl"`
	PreviewURL  *string `json:"previewUrl"`
	ExternalID  *int64  `json:"externalId"`
}

// clean ตรวจและจัดรูปข้อมูล คืนข้อความ error ถ้าไม่ผ่าน (ว่าง = ผ่าน)
func (in *songInput) clean() string {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || utf8.RuneCountInString(in.Title) > 255 {
		return "ชื่อเพลงต้องยาว 1-255 ตัวอักษร"
	}
	in.ArtistName = strings.TrimSpace(in.ArtistName)
	if in.ArtistID <= 0 && in.ArtistName == "" {
		return "ต้องระบุศิลปิน"
	}
	if utf8.RuneCountInString(in.ArtistName) > 200 {
		return "ชื่อศิลปินยาวเกิน 200 ตัวอักษร"
	}
	in.Album = blankToNil(in.Album)
	in.Genre = blankToNil(in.Genre)
	in.CoverURL = blankToNil(in.CoverURL)
	in.PreviewURL = blankToNil(in.PreviewURL)
	if in.Album != nil && utf8.RuneCountInString(*in.Album) > 255 {
		return "ชื่ออัลบั้มยาวเกิน 255 ตัวอักษร"
	}
	if in.Genre != nil && utf8.RuneCountInString(*in.Genre) > 60 {
		return "ชื่อแนวเพลงยาวเกิน 60 ตัวอักษร"
	}
	if in.Year != nil && (*in.Year < 1900 || *in.Year > 2100) {
		return "ปีต้องอยู่ระหว่าง 1900 ถึง 2100"
	}
	if in.DurationSec != nil && *in.DurationSec < 0 {
		return "ความยาวเพลงต้องไม่ติดลบ"
	}
	if !validURL(in.CoverURL) || !validURL(in.PreviewURL) {
		return "ลิงก์ต้องขึ้นต้นด้วย http:// หรือ https://"
	}
	return ""
}

// resolveArtist คืน id ศิลปินของเพลง: ถ้าส่ง artistId มาต้องมีอยู่จริง ไม่งั้นใช้ artistName (หาเจอใช้ของเดิม ไม่เจอสร้างใหม่)
// คืน (id, ข้อความ error สำหรับผู้ใช้, error ของระบบ)
func (h *Handler) resolveArtist(ctx context.Context, in songInput) (int64, string, error) {
	if in.ArtistID > 0 {
		var exists bool
		if err := h.DB.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM artists WHERE id = $1)`, in.ArtistID).Scan(&exists); err != nil {
			return 0, "", err
		}
		if !exists {
			return 0, "ไม่พบศิลปินที่ระบุ", nil
		}
		return in.ArtistID, "", nil
	}

	find := func() (int64, error) {
		var id int64
		err := h.DB.QueryRowContext(ctx, `SELECT id FROM artists WHERE lower(name) = lower($1)`, in.ArtistName).Scan(&id)
		return id, err
	}
	id, err := find()
	if err == nil {
		return id, "", nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, "", err
	}
	err = h.DB.QueryRowContext(ctx, `INSERT INTO artists (name) VALUES ($1) RETURNING id`, in.ArtistName).Scan(&id)
	if isPgCode(err, "23505") { // มีคนสร้างพร้อมกัน ลองหาใหม่
		id, err = find()
	}
	return id, "", err
}

// POST /api/admin/songs -> 201 Song
func (h *Handler) AdminCreateSong(w http.ResponseWriter, r *http.Request) {
	var in songInput
	if !response.Decode(w, r, &in) {
		return
	}
	if msg := in.clean(); msg != "" {
		response.Error(w, http.StatusBadRequest, msg)
		return
	}
	artistID, msg, err := h.resolveArtist(r.Context(), in)
	if err != nil {
		serverError(w, err)
		return
	}
	if msg != "" {
		response.Error(w, http.StatusBadRequest, msg)
		return
	}

	var id int64
	err = h.DB.QueryRowContext(r.Context(),
		`INSERT INTO songs (title, artist_id, album, genre, year, duration_sec, cover_url, preview_url, external_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
		in.Title, artistID, in.Album, in.Genre, in.Year, in.DurationSec, in.CoverURL, in.PreviewURL, in.ExternalID,
	).Scan(&id)
	if isPgCode(err, "23505") {
		response.Error(w, http.StatusConflict, "เพลงนี้ถูกนำเข้าแล้ว")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	s, err := h.getSong(r.Context(), id)
	if err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, s)
}

// PUT /api/admin/songs/{id} -> 200 Song (externalId แก้ไม่ได้)
func (h *Handler) AdminUpdateSong(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		notFound(w, "เพลงนี้")
		return
	}
	var in songInput
	if !response.Decode(w, r, &in) {
		return
	}
	if msg := in.clean(); msg != "" {
		response.Error(w, http.StatusBadRequest, msg)
		return
	}
	artistID, msg, err := h.resolveArtist(r.Context(), in)
	if err != nil {
		serverError(w, err)
		return
	}
	if msg != "" {
		response.Error(w, http.StatusBadRequest, msg)
		return
	}

	res, err := h.DB.ExecContext(r.Context(),
		`UPDATE songs SET title = $1, artist_id = $2, album = $3, genre = $4, year = $5, duration_sec = $6,
		        cover_url = $7, preview_url = $8, updated_at = now()
		 WHERE id = $9`,
		in.Title, artistID, in.Album, in.Genre, in.Year, in.DurationSec, in.CoverURL, in.PreviewURL, id)
	if err != nil {
		serverError(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		notFound(w, "เพลงนี้")
		return
	}
	s, err := h.getSong(r.Context(), id)
	if err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, s)
}

// DELETE /api/admin/songs/{id} -> 204 (เพลงในเพลย์ลิสต์และประวัติการฟังถูกลบตามด้วย)
func (h *Handler) AdminDeleteSong(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		notFound(w, "เพลงนี้")
		return
	}
	res, err := h.DB.ExecContext(r.Context(), `DELETE FROM songs WHERE id = $1`, id)
	if err != nil {
		serverError(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		notFound(w, "เพลงนี้")
		return
	}
	response.NoContent(w)
}

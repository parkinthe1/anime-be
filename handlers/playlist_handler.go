package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"musicken-be/middleware"
	"musicken-be/models"
	"musicken-be/response"
)

const (
	maxPlaylistName = 100
	maxPlaylistDesc = 500
)

func (h *Handler) loadPlaylist(ctx context.Context, id int64) (models.Playlist, error) {
	return scanPlaylist(h.DB.QueryRowContext(ctx, `SELECT `+playlistSelect+playlistFrom+` WHERE p.id = $1`, id))
}

func (h *Handler) loadPlaylistSongs(ctx context.Context, id int64) ([]models.Song, error) {
	rows, err := h.DB.QueryContext(ctx,
		`SELECT `+songSelect+`
		 FROM playlist_songs ps JOIN songs s ON s.id = ps.song_id JOIN artists ar ON ar.id = s.artist_id
		 WHERE ps.playlist_id = $1 ORDER BY ps.position, ps.added_at`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	songs := make([]models.Song, 0)
	for rows.Next() {
		s, err := scanSong(rows)
		if err != nil {
			return nil, err
		}
		songs = append(songs, s)
	}
	return songs, rows.Err()
}

func (h *Handler) detail(ctx context.Context, p models.Playlist) (models.PlaylistDetail, error) {
	songs, err := h.loadPlaylistSongs(ctx, p.ID)
	return models.PlaylistDetail{Playlist: p, Songs: songs}, err
}

func (h *Handler) listPlaylists(ctx context.Context, where string, args ...any) ([]models.Playlist, error) {
	rows, err := h.DB.QueryContext(ctx, `SELECT `+playlistSelect+playlistFrom+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]models.Playlist, 0)
	for rows.Next() {
		p, err := scanPlaylist(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return items, rows.Err()
}

// ---------- guest: เพลย์ลิสต์สาธารณะ ----------

// GET /api/playlists?q= -> { items: [Playlist] } เฉพาะที่ตั้งเป็นสาธารณะ เรียงตามยอดถูกใจ
func (h *Handler) ListPublicPlaylists(w http.ResponseWriter, r *http.Request) {
	q := likeEscaper.Replace(strings.TrimSpace(r.URL.Query().Get("q")))
	items, err := h.listPlaylists(r.Context(),
		`WHERE p.is_public AND ($1 = '' OR p.name ILIKE '%' || $1 || '%')
		 ORDER BY (SELECT COUNT(*) FROM playlist_likes l WHERE l.playlist_id = p.id) DESC, p.id DESC LIMIT 60`, q)
	if err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// GET /api/playlists/{id} -> PlaylistDetail (ลิสต์ส่วนตัวตอบ 404 เหมือนไม่มีอยู่ เจ้าของดูผ่าน /api/me/playlists/{id})
func (h *Handler) GetPublicPlaylist(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		notFound(w, "เพลย์ลิสต์นี้")
		return
	}
	p, err := h.loadPlaylist(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !p.IsPublic) {
		notFound(w, "เพลย์ลิสต์นี้")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	d, err := h.detail(r.Context(), p)
	if err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, d)
}

// ---------- member: เพลย์ลิสต์ของตัวเอง ----------
// member_id ใช้จาก token เท่านั้น ไม่รับจาก body และเพลย์ลิสต์ของคนอื่นตอบ 404

type playlistInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsPublic    bool   `json:"isPublic"`
}

func (in *playlistInput) clean() string {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > maxPlaylistName {
		return "ชื่อเพลย์ลิสต์ต้องยาว 1-100 ตัวอักษร"
	}
	if utf8.RuneCountInString(in.Description) > maxPlaylistDesc {
		return "คำอธิบายยาวเกิน 500 ตัวอักษร"
	}
	return ""
}

// owns ตรวจว่าเพลย์ลิสต์นี้เป็นของสมาชิกคนนี้จริง
func (h *Handler) owns(ctx context.Context, playlistID, memberID int64) (bool, error) {
	var ok bool
	err := h.DB.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM playlists WHERE id = $1 AND member_id = $2)`, playlistID, memberID).Scan(&ok)
	return ok, err
}

// GET /api/me/playlists -> { items }
func (h *Handler) ListMyPlaylists(w http.ResponseWriter, r *http.Request) {
	me := middleware.CurrentMember(r)
	items, err := h.listPlaylists(r.Context(), `WHERE p.member_id = $1 ORDER BY p.id DESC`, me.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// POST /api/me/playlists { name, description, isPublic } -> 201 Playlist
func (h *Handler) CreatePlaylist(w http.ResponseWriter, r *http.Request) {
	var in playlistInput
	if !response.Decode(w, r, &in) {
		return
	}
	if msg := in.clean(); msg != "" {
		response.Error(w, http.StatusBadRequest, msg)
		return
	}
	me := middleware.CurrentMember(r)
	var id int64
	if err := h.DB.QueryRowContext(r.Context(),
		`INSERT INTO playlists (member_id, name, description, is_public) VALUES ($1, $2, $3, $4) RETURNING id`,
		me.ID, in.Name, in.Description, in.IsPublic).Scan(&id); err != nil {
		serverError(w, err)
		return
	}
	p, err := h.loadPlaylist(r.Context(), id)
	if err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, p)
}

// GET /api/me/playlists/{id} -> PlaylistDetail
func (h *Handler) GetMyPlaylist(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		notFound(w, "เพลย์ลิสต์นี้")
		return
	}
	me := middleware.CurrentMember(r)
	p, err := h.loadPlaylist(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && p.OwnerID != me.ID) {
		notFound(w, "เพลย์ลิสต์นี้")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	d, err := h.detail(r.Context(), p)
	if err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, d)
}

// PUT /api/me/playlists/{id} { name, description, isPublic } -> 200 Playlist
func (h *Handler) UpdatePlaylist(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		notFound(w, "เพลย์ลิสต์นี้")
		return
	}
	var in playlistInput
	if !response.Decode(w, r, &in) {
		return
	}
	if msg := in.clean(); msg != "" {
		response.Error(w, http.StatusBadRequest, msg)
		return
	}
	me := middleware.CurrentMember(r)
	res, err := h.DB.ExecContext(r.Context(),
		`UPDATE playlists SET name = $1, description = $2, is_public = $3, updated_at = now()
		 WHERE id = $4 AND member_id = $5`, in.Name, in.Description, in.IsPublic, id, me.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		notFound(w, "เพลย์ลิสต์นี้")
		return
	}
	p, err := h.loadPlaylist(r.Context(), id)
	if err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, p)
}

// DELETE /api/me/playlists/{id} -> 204
func (h *Handler) DeletePlaylist(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		notFound(w, "เพลย์ลิสต์นี้")
		return
	}
	me := middleware.CurrentMember(r)
	res, err := h.DB.ExecContext(r.Context(), `DELETE FROM playlists WHERE id = $1 AND member_id = $2`, id, me.ID)
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

// PUT /api/me/playlists/{id}/songs/{songId} -> 204 เพิ่มต่อท้ายลิสต์ (เพิ่มซ้ำไม่ error)
func (h *Handler) AddSongToPlaylist(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	songID, ok2 := parseIDParam(r, "songId")
	if !ok || !ok2 {
		notFound(w, "เพลย์ลิสต์หรือเพลงนี้")
		return
	}
	me := middleware.CurrentMember(r)
	mine, err := h.owns(r.Context(), id, me.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	if !mine {
		notFound(w, "เพลย์ลิสต์นี้")
		return
	}

	_, err = h.DB.ExecContext(r.Context(),
		`INSERT INTO playlist_songs (playlist_id, song_id, position)
		 SELECT $1::bigint, $2::bigint, COALESCE(MAX(position), 0) + 1 FROM playlist_songs WHERE playlist_id = $1::bigint
		 ON CONFLICT DO NOTHING`, id, songID)
	if isPgCode(err, "23503") { // ไม่มีเพลงนี้
		notFound(w, "เพลงนี้")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	response.NoContent(w)
}

// DELETE /api/me/playlists/{id}/songs/{songId} -> 204
func (h *Handler) RemoveSongFromPlaylist(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	songID, ok2 := parseIDParam(r, "songId")
	if !ok || !ok2 {
		notFound(w, "เพลย์ลิสต์หรือเพลงนี้")
		return
	}
	me := middleware.CurrentMember(r)
	mine, err := h.owns(r.Context(), id, me.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	if !mine {
		notFound(w, "เพลย์ลิสต์นี้")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`DELETE FROM playlist_songs WHERE playlist_id = $1 AND song_id = $2`, id, songID); err != nil {
		serverError(w, err)
		return
	}
	response.NoContent(w)
}

// PUT /api/me/playlists/{id}/order { songIds: [..] } -> 200 PlaylistDetail
// songIds ต้องมีครบทุกเพลงในลิสต์ ไม่ซ้ำ ไม่เกิน ตามลำดับใหม่
func (h *Handler) ReorderPlaylist(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		notFound(w, "เพลย์ลิสต์นี้")
		return
	}
	var in struct {
		SongIDs []int64 `json:"songIds"`
	}
	if !response.Decode(w, r, &in) {
		return
	}
	me := middleware.CurrentMember(r)
	mine, err := h.owns(r.Context(), id, me.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	if !mine {
		notFound(w, "เพลย์ลิสต์นี้")
		return
	}

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		serverError(w, err)
		return
	}
	defer tx.Rollback() // หลัง Commit สำเร็จ Rollback จะไม่ทำอะไร

	rows, err := tx.QueryContext(r.Context(), `SELECT song_id FROM playlist_songs WHERE playlist_id = $1`, id)
	if err != nil {
		serverError(w, err)
		return
	}
	current := map[int64]bool{}
	for rows.Next() {
		var sid int64
		if err := rows.Scan(&sid); err != nil {
			rows.Close()
			serverError(w, err)
			return
		}
		current[sid] = true
	}
	rows.Close() // ต้องปิดก่อนสั่งคำสั่งถัดไปใน transaction เดียวกัน
	if err := rows.Err(); err != nil {
		serverError(w, err)
		return
	}

	seen := map[int64]bool{}
	valid := len(in.SongIDs) == len(current)
	for _, sid := range in.SongIDs {
		if !current[sid] || seen[sid] {
			valid = false
			break
		}
		seen[sid] = true
	}
	if !valid {
		response.Error(w, http.StatusBadRequest, "รายการเพลงต้องตรงกับเพลงที่อยู่ในเพลย์ลิสต์ทุกเพลง ไม่ซ้ำ ไม่ขาด ไม่เกิน")
		return
	}

	for i, sid := range in.SongIDs {
		if _, err := tx.ExecContext(r.Context(),
			`UPDATE playlist_songs SET position = $1 WHERE playlist_id = $2 AND song_id = $3`, i+1, id, sid); err != nil {
			serverError(w, err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		serverError(w, err)
		return
	}

	p, err := h.loadPlaylist(r.Context(), id)
	if err != nil {
		serverError(w, err)
		return
	}
	d, err := h.detail(r.Context(), p)
	if err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, d)
}

// ---------- member: ถูกใจ ----------

func (h *Handler) likeCount(ctx context.Context, playlistID int64) (int, error) {
	var n int
	err := h.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM playlist_likes WHERE playlist_id = $1`, playlistID).Scan(&n)
	return n, err
}

// PUT /api/playlists/{id}/like -> 200 { liked: true, likeCount } (ถูกใจได้เฉพาะลิสต์สาธารณะ หรือของตัวเอง)
func (h *Handler) LikePlaylist(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		notFound(w, "เพลย์ลิสต์นี้")
		return
	}
	me := middleware.CurrentMember(r)
	p, err := h.loadPlaylist(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !p.IsPublic && p.OwnerID != me.ID) {
		notFound(w, "เพลย์ลิสต์นี้")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO playlist_likes (member_id, playlist_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, me.ID, id); err != nil {
		serverError(w, err)
		return
	}
	n, err := h.likeCount(r.Context(), id)
	if err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"liked": true, "likeCount": n})
}

// DELETE /api/playlists/{id}/like -> 200 { liked: false, likeCount }
func (h *Handler) UnlikePlaylist(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		notFound(w, "เพลย์ลิสต์นี้")
		return
	}
	me := middleware.CurrentMember(r)
	if _, err := h.DB.ExecContext(r.Context(),
		`DELETE FROM playlist_likes WHERE member_id = $1 AND playlist_id = $2`, me.ID, id); err != nil {
		serverError(w, err)
		return
	}
	n, err := h.likeCount(r.Context(), id)
	if err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"liked": false, "likeCount": n})
}

// GET /api/me/likes -> { items: [Playlist] } เพลย์ลิสต์ที่ถูกใจ (ตัวที่เจ้าของเปลี่ยนเป็นส่วนตัวภายหลังจะไม่แสดง)
func (h *Handler) ListMyLikes(w http.ResponseWriter, r *http.Request) {
	me := middleware.CurrentMember(r)
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT `+playlistSelect+`
		 FROM playlist_likes lk
		 JOIN playlists p ON p.id = lk.playlist_id
		 JOIN members m ON m.id = p.member_id
		 WHERE lk.member_id = $1 AND (p.is_public OR p.member_id = $1)
		 ORDER BY lk.created_at DESC`, me.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()
	items := make([]models.Playlist, 0)
	for rows.Next() {
		p, err := scanPlaylist(rows)
		if err != nil {
			serverError(w, err)
			return
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// ---------- member: ประวัติการฟัง ----------

type historyItem struct {
	models.Song
	PlayedAt time.Time `json:"playedAt"`
}

// POST /api/songs/{id}/play -> 204 บันทึกว่าสมาชิกกดฟังเพลงนี้
func (h *Handler) RecordPlay(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		notFound(w, "เพลงนี้")
		return
	}
	me := middleware.CurrentMember(r)
	_, err := h.DB.ExecContext(r.Context(), `INSERT INTO plays (member_id, song_id) VALUES ($1, $2)`, me.ID, id)
	if isPgCode(err, "23503") {
		notFound(w, "เพลงนี้")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	response.NoContent(w)
}

// GET /api/me/history -> { items: [Song + playedAt] } 50 รายการล่าสุด
func (h *Handler) ListHistory(w http.ResponseWriter, r *http.Request) {
	me := middleware.CurrentMember(r)
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT `+songSelect+`, pl.played_at
		 FROM plays pl JOIN songs s ON s.id = pl.song_id JOIN artists ar ON ar.id = s.artist_id
		 WHERE pl.member_id = $1 ORDER BY pl.played_at DESC, pl.id DESC LIMIT 50`, me.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()
	items := make([]historyItem, 0)
	for rows.Next() {
		var at time.Time
		s, err := scanSong(rows, &at)
		if err != nil {
			serverError(w, err)
			return
		}
		items = append(items, historyItem{Song: s, PlayedAt: at})
	}
	if err := rows.Err(); err != nil {
		serverError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"items": items})
}

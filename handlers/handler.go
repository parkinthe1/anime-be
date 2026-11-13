// Package handlers: ตัวจัดการแต่ละ endpoint ของ API (แยกไฟล์ตามกลุ่ม: auth / me / song / artist / playlist / admin)
package handlers

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"musicken-be/auth"
	"musicken-be/models"
	"musicken-be/response"

	"github.com/jackc/pgx/v5/pgconn"
)

type Handler struct {
	DB     *sql.DB
	Tokens *auth.TokenService
}

const (
	minPasswordLen = 4  // ตรงกับ docs/api-contract.md ของ FE
	maxPasswordLen = 72 // ขีดจำกัดของ bcrypt
	minNameLen     = 2
)

type rowScanner interface {
	Scan(dest ...any) error
}

const memberColumns = "id, email, display_name, role, created_at"

func scanMember(row rowScanner) (models.Member, error) {
	var m models.Member
	err := row.Scan(&m.ID, &m.Email, &m.DisplayName, &m.Role, &m.CreatedAt)
	return m, err
}

// ---------- เพลง ----------

const songSelect = "s.id, s.title, s.artist_id, ar.name, s.album, s.genre, s.year, s.duration_sec, s.cover_url, s.preview_url, s.external_id"
const songFrom = " FROM songs s JOIN artists ar ON ar.id = s.artist_id "

// scanSong อ่านคอลัมน์ตาม songSelect ถ้ามีคอลัมน์เพิ่มต่อท้าย (เช่น COUNT(*) OVER()) ส่งตัวรับมาใน extra
func scanSong(row rowScanner, extra ...any) (models.Song, error) {
	var s models.Song
	dest := []any{&s.ID, &s.Title, &s.ArtistID, &s.ArtistName, &s.Album, &s.Genre, &s.Year, &s.DurationSec, &s.CoverURL, &s.PreviewURL, &s.ExternalID}
	dest = append(dest, extra...)
	err := row.Scan(dest...)
	return s, err
}

// ---------- เพลย์ลิสต์ ----------

const playlistSelect = `p.id, p.name, p.description, p.is_public, p.member_id, m.display_name, p.created_at,
	(SELECT COUNT(*) FROM playlist_songs ps WHERE ps.playlist_id = p.id),
	(SELECT COUNT(*) FROM playlist_likes l WHERE l.playlist_id = p.id)`
const playlistFrom = " FROM playlists p JOIN members m ON m.id = p.member_id "

func scanPlaylist(row rowScanner) (models.Playlist, error) {
	var p models.Playlist
	err := row.Scan(&p.ID, &p.Name, &p.Description, &p.IsPublic, &p.OwnerID, &p.OwnerName, &p.CreatedAt, &p.SongCount, &p.LikeCount)
	return p, err
}

// ---------- ตัวช่วยทั่วไป ----------

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// parseIDParam อ่านตัวแปรใน URL ชื่อ name (เช่น {id}, {songId}) ถ้าไม่ใช่เลขบวกถือว่าไม่พบ
func parseIDParam(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	return id, err == nil && id > 0
}

func parseID(r *http.Request) (int64, bool) { return parseIDParam(r, "id") }

// intQuery อ่านเลขจาก query string ถ้าไม่มีหรืออ่านไม่ได้ใช้ค่า def และบีบให้อยู่ระหว่าง min-max
func intQuery(r *http.Request, key string, def, min, max int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil {
		return def
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// isPgCode ตรวจรหัส error ของ PostgreSQL: 23505 = ซ้ำ (unique), 23503 = อ้างอิงไม่มี/ยังถูกอ้างอิง (foreign key)
func isPgCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

func serverError(w http.ResponseWriter, err error) {
	log.Printf("error: %v", err)
	response.Error(w, http.StatusInternalServerError, "เซิร์ฟเวอร์ผิดพลาด ลองใหม่อีกครั้ง")
}

func notFound(w http.ResponseWriter, what string) {
	response.Error(w, http.StatusNotFound, "ไม่พบ"+what)
}

func normalizeEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// validatePassword คืนข้อความ error ถ้าไม่ผ่าน (ว่าง = ผ่าน)
func validatePassword(p string) string {
	if len([]rune(p)) < minPasswordLen {
		return "รหัสผ่านต้องยาวอย่างน้อย 4 ตัวอักษร"
	}
	if len(p) > maxPasswordLen {
		return "รหัสผ่านยาวเกินไป (ไม่เกิน 72 ไบต์)"
	}
	return ""
}

func blankToNil(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}

// validURL: nil ผ่าน (ไม่บังคับ) ถ้ามีต้องเป็น http/https เท่านั้น (กัน javascript: ที่หน้าเว็บเอาไปใส่ src/href)
func validURL(s *string) bool {
	if s == nil {
		return true
	}
	u, err := url.Parse(*s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

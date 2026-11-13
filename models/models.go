// Package models เก็บรูปร่างข้อมูลที่ตอบกลับหน้าเว็บ (ชื่อ field เป็น camelCase ตาม docs/api-contract.md ของ FE)
package models

import "time"

// Member ไม่มี password_hash เพื่อไม่ให้หลุดไปกับ JSON โดยบังเอิญ
type Member struct {
	ID          int64     `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"displayName"`
	Role        string    `json:"role"` // "member" | "admin"
	CreatedAt   time.Time `json:"createdAt"`
}

type Artist struct {
	ID        int64   `json:"id"`
	Name      string  `json:"name"`
	Bio       *string `json:"bio"`
	ImageURL  *string `json:"imageUrl"`
	SongCount int     `json:"songCount"`
}

type Song struct {
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	ArtistID    int64   `json:"artistId"`
	ArtistName  string  `json:"artistName"`
	Album       *string `json:"album"`
	Genre       *string `json:"genre"`
	Year        *int    `json:"year"`
	DurationSec *int    `json:"durationSec"`
	CoverURL    *string `json:"coverUrl"`
	PreviewURL  *string `json:"previewUrl"`
	ExternalID  *int64  `json:"externalId"`
}

type Playlist struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsPublic    bool      `json:"isPublic"`
	OwnerID     int64     `json:"ownerId"`
	OwnerName   string    `json:"ownerName"`
	CreatedAt   time.Time `json:"createdAt"`
	SongCount   int       `json:"songCount"`
	LikeCount   int       `json:"likeCount"`
}

// PlaylistDetail = Playlist + เพลงเรียงตามลำดับ
type PlaylistDetail struct {
	Playlist
	Songs []Song `json:"songs"`
}

const (
	RoleMember = "member"
	RoleAdmin  = "admin"
)

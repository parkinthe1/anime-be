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

type Anime struct {
	ID       int64    `json:"id"`
	Title    string   `json:"title"`
	TitleTh  *string  `json:"titleTh"`
	Year     *int     `json:"year"`
	Genre    *string  `json:"genre"`
	Rating   *float64 `json:"rating"`
	Episodes *int     `json:"episodes"`
	Status   string   `json:"status"` // "airing" | "finished" | "upcoming"
	Poster   *string  `json:"poster"`
	Synopsis string   `json:"synopsis"`
}

const (
	RoleMember = "member"
	RoleAdmin  = "admin"
)

var ValidStatuses = map[string]bool{"airing": true, "finished": true, "upcoming": true}

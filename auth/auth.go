// Package auth: เข้ารหัสผ่านด้วย bcrypt และออก/ตรวจ token แบบ JWT (HS256)
package auth

import (
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

type TokenService struct {
	secret []byte
	ttl    time.Duration
}

func NewTokenService(secret string, ttl time.Duration) *TokenService {
	return &TokenService{secret: []byte(secret), ttl: ttl}
}

// Issue ออก token ที่เก็บแค่ id สมาชิก (บทบาทไม่ใส่ใน token: ให้ middleware ไปอ่านจาก DB ทุกครั้ง เปลี่ยนบทบาทแล้วมีผลทันที)
func (t *TokenService) Issue(memberID int64) (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(memberID, 10),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(t.ttl)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
}

// Parse ตรวจลายเซ็นและวันหมดอายุ แล้วคืน id สมาชิก
func (t *TokenService) Parse(token string) (int64, error) {
	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims,
		func(*jwt.Token) (any, error) { return t.secret, nil },
		jwt.WithValidMethods([]string{"HS256"}),
	)
	if err != nil || !parsed.Valid {
		return 0, errors.New("token ไม่ถูกต้องหรือหมดอายุ")
	}
	return strconv.ParseInt(claims.Subject, 10, 64)
}

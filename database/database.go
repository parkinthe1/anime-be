// Package database เชื่อม PostgreSQL (Supabase) ด้วย pgx ผ่าน database/sql
package database

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"musicken-be/auth"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func Connect(url string) (*sql.DB, error) {
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	// simple protocol ใช้ได้ทั้ง Session pooler (5432) และ Transaction pooler (6543) ของ Supabase
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	db := stdlib.OpenDB(*cfg)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// EnsureAdmin สร้างบัญชี admin ถ้ายังไม่มีอีเมลนี้ในระบบ คืน true ถ้าสร้างใหม่
func EnsureAdmin(db *sql.DB, email, password string) (bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	hash, err := auth.HashPassword(password)
	if err != nil {
		return false, err
	}
	res, err := db.Exec(`
		INSERT INTO members (email, password_hash, display_name, role)
		SELECT $1::text, $2::text, 'Admin', 'admin'
		WHERE NOT EXISTS (SELECT 1 FROM members WHERE lower(email) = $1::text)`,
		email, hash)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

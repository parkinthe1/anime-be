// MusicKen API: backend (Go) ของเว็บเพลง MusicKen ต่อกับ FE (React) ตาม docs/api-contract.md
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"musicken-be/auth"
	"musicken-be/config"
	"musicken-be/database"
	"musicken-be/handlers"
	"musicken-be/middleware"
	"musicken-be/router"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("เชื่อมฐานข้อมูลไม่ได้: %v", err)
	}
	defer db.Close()

	if cfg.AdminEmail != "" && cfg.AdminPassword != "" {
		created, err := database.EnsureAdmin(db, cfg.AdminEmail, cfg.AdminPassword)
		if err != nil {
			log.Fatalf("สร้าง admin ไม่ได้ (รัน db/schema.sql แล้วหรือยัง?): %v", err)
		}
		if created {
			log.Printf("สร้างบัญชี admin: %s", cfg.AdminEmail)
		}
	}

	tokens := auth.NewTokenService(cfg.JWTSecret, 24*time.Hour)
	h := &handlers.Handler{DB: db, Tokens: tokens}
	authn := &middleware.Authenticator{DB: db, Tokens: tokens}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router.New(h, authn, cfg.CORSOrigins),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("MusicKen API พร้อมที่พอร์ต %s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

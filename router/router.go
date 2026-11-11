// Package router ประกาศเส้นทางทั้งหมดและบอกว่าแต่ละเส้นทางต้องมีสิทธิ์อะไร
// (guest = ไม่ครอบอะไร, member = RequireLogin, admin = RequireAdmin)
package router

import (
	"net/http"

	"aniwave-be/handlers"
	"aniwave-be/middleware"
	"aniwave-be/response"
)

func New(h *handlers.Handler, a *middleware.Authenticator, corsOrigins []string) http.Handler {
	mux := http.NewServeMux()

	member := func(f http.HandlerFunc) http.Handler { return a.RequireLogin(f) }
	admin := func(f http.HandlerFunc) http.Handler { return a.RequireAdmin(f) }

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		response.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// ---------- guest ----------
	mux.HandleFunc("POST /api/auth/register", h.Register)
	mux.HandleFunc("POST /api/auth/login", h.Login)
	mux.HandleFunc("GET /api/anime", h.ListAnime)
	mux.HandleFunc("GET /api/anime/{id}", h.GetAnime)
	mux.HandleFunc("GET /api/anime/{id}/reviews", h.ListReviews)

	// ---------- member ----------
	mux.Handle("GET /api/me", member(h.GetMe))
	mux.Handle("PUT /api/me", member(h.UpdateMe))
	mux.Handle("PUT /api/me/password", member(h.ChangePassword))
	mux.Handle("POST /api/anime/{id}/reviews", member(h.CreateReview))
	mux.Handle("PUT /api/anime/{id}/vote", member(h.PutVote))
	mux.Handle("GET /api/me/wishlist", member(h.ListWishlist))
	mux.Handle("PUT /api/me/wishlist/{id}", member(h.AddWishlist))
	mux.Handle("DELETE /api/me/wishlist/{id}", member(h.RemoveWishlist))

	// ---------- admin ----------
	mux.Handle("GET /api/admin/stats", admin(h.AdminStats))
	mux.Handle("GET /api/admin/members", admin(h.AdminMembers))
	mux.Handle("PUT /api/admin/members/{id}/role", admin(h.AdminSetRole))
	mux.Handle("GET /api/admin/anime/{id}", admin(h.AdminGetAnime))
	mux.Handle("POST /api/admin/anime", admin(h.AdminCreateAnime))
	mux.Handle("PUT /api/admin/anime/{id}", admin(h.AdminUpdateAnime))
	mux.Handle("DELETE /api/admin/anime/{id}", admin(h.AdminDeleteAnime))

	// เส้นทางที่ไม่มีอยู่ ตอบเป็น JSON เหมือน endpoint อื่น
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		response.Error(w, http.StatusNotFound, "ไม่พบเส้นทางนี้")
	})

	// ลำดับชั้น: log -> กัน panic -> CORS -> เส้นทาง
	return middleware.Logger(middleware.Recover(middleware.CORS(corsOrigins)(mux)))
}

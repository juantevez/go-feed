package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/juantevez/my-ig/social-service/internal/infrastructure/http/handler"
	socialmw "github.com/juantevez/my-ig/social-service/internal/infrastructure/http/middleware"
)

func New(follow *handler.FollowHandler, jwtSecret string) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Rutas públicas — estadísticas y listas de followers son visibles sin auth
	r.Get("/users/{user_id}/followers", follow.GetFollowers)
	r.Get("/users/{user_id}/following", follow.GetFollowing)
	r.Get("/users/{user_id}/stats", follow.GetStats)

	// Rutas protegidas — requieren JWT
	r.Group(func(r chi.Router) {
		r.Use(socialmw.JWTMiddleware(jwtSecret))
		r.Post("/follow/{target_id}", follow.Follow)
		r.Delete("/follow/{target_id}", follow.Unfollow)
	})

	return r
}

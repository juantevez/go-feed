package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/juantevez/my-ig/auth-service/internal/infrastructure/http/handler"
)

func New(auth *handler.AuthHandler) http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Health
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		// TODO: check DB + NATS connectivity
		w.WriteHeader(http.StatusOK)
	})

	// Auth routes (no JWT required)
	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", auth.Register)
		r.Post("/login", auth.Login)
		r.Post("/refresh", auth.Refresh)

		// Logout requires a valid access token
		// TODO: add JWT middleware here
		r.Post("/logout", auth.Logout)
	})

	return r
}

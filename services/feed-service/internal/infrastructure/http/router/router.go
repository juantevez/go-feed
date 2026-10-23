package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/juantevez/my-ig/feed-service/internal/infrastructure/http/handler"
	feedmw "github.com/juantevez/my-ig/feed-service/internal/infrastructure/http/middleware"
)

func New(feed *handler.FeedHandler, jwtSecret string, revoked feedmw.RevocationChecker) http.Handler {
	r := chi.NewRouter()

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		// TODO: check DB + NATS connectivity
		w.WriteHeader(http.StatusOK)
	})

	// Feed del usuario autenticado: el user_id sale del JWT (claim sub).
	r.Group(func(r chi.Router) {
		r.Use(feedmw.JWTMiddleware(jwtSecret, revoked))
		r.Get("/feed", feed.GetFeed)
	})

	return r
}

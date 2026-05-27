package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/juantevez/my-ig/feed-service/internal/infrastructure/http/handler"
)

func New(feed *handler.FeedHandler) http.Handler {
	r := chi.NewRouter()

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

	// Feed — el X-User-ID lo inyecta el API Gateway desde el JWT validado.
	r.Get("/feed", feed.GetFeed)

	return r
}

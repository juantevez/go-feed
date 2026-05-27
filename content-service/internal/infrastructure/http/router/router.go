package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/juantevez/my-ig/content-service/internal/infrastructure/http/handler"
	contentmw "github.com/juantevez/my-ig/content-service/internal/infrastructure/http/middleware"
)

func New(posts *handler.PostHandler, jwtSecret string) http.Handler {
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

	// GET /posts/{id} — público (sin JWT, visibilidad manejada en el use case)
	r.Get("/posts/{id}", posts.GetPost)

	// Rutas protegidas — requieren JWT válido de auth-service
	r.Group(func(r chi.Router) {
		r.Use(contentmw.JWTMiddleware(jwtSecret))
		r.Post("/posts", posts.CreatePost)
		r.Delete("/posts/{id}", posts.DeletePost)
	})

	return r
}

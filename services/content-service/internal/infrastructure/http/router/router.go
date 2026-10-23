package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/juantevez/my-ig/content-service/internal/infrastructure/http/handler"
	contentmw "github.com/juantevez/my-ig/content-service/internal/infrastructure/http/middleware"
)

func New(posts *handler.PostHandler, jwtSecret string, revoked contentmw.RevocationChecker) http.Handler {
	r := chi.NewRouter()

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-User-ID"},
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
		w.WriteHeader(http.StatusOK)
	})

	// GET /posts/{id} — público; el JWT es opcional y solo sirve para que el autor
	// vea sus posts no públicos (visibilidad manejada en el use case)
	r.With(contentmw.OptionalJWTMiddleware(jwtSecret, revoked)).Get("/posts/{id}", posts.GetPost)

	// Rutas protegidas — requieren JWT válido de auth-service
	r.Group(func(r chi.Router) {
		r.Use(contentmw.JWTMiddleware(jwtSecret, revoked))
		r.Post("/posts", posts.CreatePost)
		r.Get("/users/{id}/posts", posts.ListUserPosts)
		r.Delete("/posts/{id}", posts.DeletePost)
	})

	return r
}

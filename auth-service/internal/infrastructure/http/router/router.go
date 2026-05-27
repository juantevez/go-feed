package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	domaintoken "github.com/juantevez/my-ig/auth-service/internal/domain/token"
	"github.com/juantevez/my-ig/auth-service/internal/infrastructure/http/handler"
	"github.com/juantevez/my-ig/auth-service/internal/infrastructure/http/middleware"
	//"github.com/juantevez/my-ig/auth-service/internal/infrastructure/http/middleware"
)

func New(auth *handler.AuthHandler, tokenSvc domaintoken.Service) http.Handler {
	r := chi.NewRouter()

	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", auth.Register)
		r.Post("/login", auth.Login)
		r.Post("/refresh", auth.Refresh)

		r.Group(func(r chi.Router) {
			r.Use(middleware.JWTMiddleware(tokenSvc))
			r.Post("/logout", auth.Logout)
		})
	})

	return r
}

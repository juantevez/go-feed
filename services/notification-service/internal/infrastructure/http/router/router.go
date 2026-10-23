package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/juantevez/my-ig/notification-service/internal/infrastructure/http/handler"
	notifmw "github.com/juantevez/my-ig/notification-service/internal/infrastructure/http/middleware"
	"github.com/juantevez/my-ig/notification-service/internal/infrastructure/ws"
)

func New(notif *handler.NotificationHandler, jwtSecret string, revoked notifmw.RevocationChecker) http.Handler {
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

	// Todas las rutas requieren JWT
	r.Group(func(r chi.Router) {
		r.Use(notifmw.JWTMiddleware(jwtSecret, revoked))

		r.Get("/notifications", notif.GetNotifications)
		r.Patch("/notifications/{id}/read", notif.MarkRead)
		r.Patch("/notifications/read-all", notif.MarkAllRead)
	})

	// WebSocket — upgrade desde HTTP con JWT validado. El navegador no puede
	// mandar Authorization en el upgrade: manda el token como subprotocolo.
	r.With(notifmw.JWTWebSocketMiddleware(jwtSecret, revoked, ws.TokenSubprotocol)).
		Get("/ws", notif.ServeWS)

	return r
}

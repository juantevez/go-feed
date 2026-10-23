package user

import (
	"time"

	"github.com/juantevez/my-ig/shared/events"
)

// NewRegisteredEnvelope construye el Envelope para auth.user.registered.v1.
// Nunca incluye datos sensibles (sin password hash, sin tokens).
func NewRegisteredEnvelope(u *User, traceID string) events.Envelope {
	payload := events.UserRegisteredPayload{
		UserID:    u.ID,
		Username:  u.Username,
		Email:     u.Email,
		Role:      string(u.Role),
		OccuredAt: u.CreatedAt,
	}
	return events.Wrap(
		events.TopicAuthUserRegistered,
		"auth-service",
		"register:"+u.ID.String(),
		traceID,
		payload,
	)
}

// NewLoggedInEnvelope construye el Envelope para auth.user.logged_in.v1.
// Solo metadata de la sesión — sin tokens ni credenciales.
func NewLoggedInEnvelope(u *User, traceID string) events.Envelope {
	payload := events.UserLoggedInPayload{
		UserID:    u.ID,
		OccuredAt: time.Now().UTC(),
	}
	return events.Wrap(events.TopicAuthUserLoggedIn, "auth-service", "", traceID, payload)
}

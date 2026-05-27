package events

import (
	"time"

	"github.com/google/uuid"
)

// ── auth.user.registered.v1 ───────────────────────────────────────────────────

// UserRegisteredPayload es el payload del evento TopicAuthUserRegistered.
// Nunca contiene datos sensibles (sin password hash, sin tokens).
// Los consumidores usan UserID como referencia para consultar más datos si necesitan.
type UserRegisteredPayload struct {
	UserID    uuid.UUID `json:"user_id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	OccuredAt time.Time `json:"occured_at"`
}

// ── auth.user.logged_in.v1 ────────────────────────────────────────────────────

// UserLoggedInPayload es el payload del evento TopicAuthUserLoggedIn.
// Solo metadata de la sesión — sin tokens ni credenciales.
type UserLoggedInPayload struct {
	UserID    uuid.UUID `json:"user_id"`
	OccuredAt time.Time `json:"occured_at"`
}

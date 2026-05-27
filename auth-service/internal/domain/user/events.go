package user

import (
	"time"

	"github.com/google/uuid"
)

// Topic es el nombre del tópico NATS para este evento.
// Sigue la convención: domain.entity.action.version
const TopicRegistered = "auth.user.registered.v1"

// RegisteredEvent es el payload publicado al bus cuando un usuario se registra.
// Nunca contiene datos sensibles (sin password hash, sin tokens).
type RegisteredEvent struct {
	EventID   uuid.UUID `json:"event_id"`
	UserID    uuid.UUID `json:"user_id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	OccuredAt time.Time `json:"occured_at"`
}

// NewRegisteredEvent construye el evento a partir del agregado.
func NewRegisteredEvent(u *User) RegisteredEvent {
	return RegisteredEvent{
		EventID:   uuid.New(),
		UserID:    u.ID,
		Username:  u.Username,
		Email:     u.Email,
		Role:      string(u.Role),
		OccuredAt: u.CreatedAt,
	}
}

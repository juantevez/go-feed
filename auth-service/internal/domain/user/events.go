package user

import (
	"time"

	"github.com/google/uuid"
)

// Tópicos NATS — convención: domain.entity.action.version
const (
	TopicRegistered = "auth.user.registered.v1"
	TopicLoggedIn   = "auth.user.logged_in.v1"
)

// RegisteredEvent se publica cuando un usuario se registra exitosamente.
type RegisteredEvent struct {
	EventID   uuid.UUID `json:"event_id"`
	UserID    uuid.UUID `json:"user_id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	OccuredAt time.Time `json:"occured_at"`
}

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

// LoggedInEvent se publica cuando un usuario hace login exitosamente.
// No contiene tokens ni credenciales — solo metadata de la sesión.
type LoggedInEvent struct {
	EventID   uuid.UUID `json:"event_id"`
	UserID    uuid.UUID `json:"user_id"`
	OccuredAt time.Time `json:"occured_at"`
}

func NewLoggedInEvent(u *User) LoggedInEvent {
	return LoggedInEvent{
		EventID:   uuid.New(),
		UserID:    u.ID,
		OccuredAt: time.Now().UTC(),
	}
}

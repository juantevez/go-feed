package user

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository es el directorio local de usuarios del social-service, alimentado
// por auth.user.registered.v1. Permite validar que el usuario a seguir existe
// sin consultar a auth-service.
type Repository interface {
	// Save registra un usuario. Idempotente: una reentrega del evento no falla.
	Save(ctx context.Context, id uuid.UUID, username string, registeredAt time.Time) error

	// Exists indica si el usuario está registrado.
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
}

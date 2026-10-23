package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/juantevez/my-ig/social-service/internal/domain/user"
)

// UserService mantiene el directorio local de usuarios a partir de los eventos
// de auth-service.
type UserService struct {
	users user.Repository
}

func NewUserService(users user.Repository) *UserService {
	return &UserService{users: users}
}

// RegisterUser aplica un auth.user.registered.v1. Idempotente.
func (s *UserService) RegisterUser(ctx context.Context, id uuid.UUID, username string, registeredAt time.Time) error {
	if id == uuid.Nil {
		return fmt.Errorf("register_user: user_id is required")
	}
	if err := s.users.Save(ctx, id, username, registeredAt); err != nil {
		return fmt.Errorf("register_user: %w", err)
	}
	return nil
}

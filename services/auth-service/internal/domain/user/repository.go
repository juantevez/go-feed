package user

import (
	"context"

	"github.com/google/uuid"
)

// Repository is the driven port (secondary) for user persistence.
// Any adapter (Postgres, in-memory, etc.) must satisfy this interface.
type Repository interface {
	Save(ctx context.Context, u *User) error
	FindByID(ctx context.Context, id uuid.UUID) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindByUsername(ctx context.Context, username string) (*User, error)
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	Update(ctx context.Context, u *User) error
}

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/juantevez/my-ig/shared/outbox"
)

// UserRepository implementa user.Repository sobre la tabla users (directorio
// local alimentado por auth.user.registered.v1).
type UserRepository struct {
	db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Save(ctx context.Context, id uuid.UUID, username string, registeredAt time.Time) error {
	const q = `
		INSERT INTO users (id, username, registered_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (id) DO NOTHING`
	if _, err := outbox.Conn(ctx, r.db).Exec(ctx, q, id, username, registeredAt); err != nil {
		return fmt.Errorf("user_repo.Save: %w", err)
	}
	return nil
}

func (r *UserRepository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	const q = `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)`
	var exists bool
	if err := outbox.Conn(ctx, r.db).QueryRow(ctx, q, id).Scan(&exists); err != nil {
		return false, fmt.Errorf("user_repo.Exists: %w", err)
	}
	return exists, nil
}

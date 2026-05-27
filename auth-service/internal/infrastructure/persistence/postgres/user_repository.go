package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tuusuario/my-ig/auth-service/internal/domain/user"
)

// UserRepository is the Postgres adapter for user.Repository.
type UserRepository struct {
	db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) *UserRepository {
	return &UserRepository{db: db}
}

// TODO: implement each method using pgx queries against the users table.

func (r *UserRepository) Save(ctx context.Context, u *user.User) error {
	panic("not implemented")
}

func (r *UserRepository) FindByID(ctx context.Context, id uuid.UUID) (*user.User, error) {
	panic("not implemented")
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	panic("not implemented")
}

func (r *UserRepository) FindByUsername(ctx context.Context, username string) (*user.User, error) {
	panic("not implemented")
}

func (r *UserRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	panic("not implemented")
}

func (r *UserRepository) Update(ctx context.Context, u *user.User) error {
	panic("not implemented")
}

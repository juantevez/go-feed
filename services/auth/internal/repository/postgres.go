package repository

import (
	"context"
	"fmt"

	"feed-backend/services/auth/internal/domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresRepo(ctx context.Context, dsn string) (*PostgresRepo, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool new: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("pgxpool ping: %w", err)
	}
	return &PostgresRepo{pool: pool}, nil
}

func (r *PostgresRepo) Create(ctx context.Context, u *domain.User, hashed []byte) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO users (id, username, email, password_hash)
		VALUES ($1, $2, $3, $4)
	`, u.ID, u.Username, u.Email, hashed)
	return err
}

func (r *PostgresRepo) FindByEmail(ctx context.Context, email string) (*struct {
	ID, Username, Email string
	Password            []byte
}, error) {
	var u struct {
		ID, Username, Email string
		Password            []byte
	}
	err := r.pool.QueryRow(ctx, `
		SELECT id, username, email, password_hash FROM users WHERE email = $1 AND deleted_at IS NULL
	`, email).Scan(&u.ID, &u.Username, &u.Email, &u.Password)
	if err != nil {
		return nil, fmt.Errorf("query user: %w", err)
	}
	return &u, nil
}

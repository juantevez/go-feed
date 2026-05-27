package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/juantevez/my-ig/auth-service/internal/domain/user"
)

// UserRepository is the Postgres adapter for user.Repository.
type UserRepository struct {
	db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) *UserRepository {
	return &UserRepository{db: db}
}

// Save inserts a new user row. Returns user.ErrEmailAlreadyTaken on unique violation.
func (r *UserRepository) Save(ctx context.Context, u *user.User) error {
	const q = `
		INSERT INTO users (id, username, email, password_hash, role, is_verified, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err := r.db.Exec(ctx, q,
		u.ID,
		u.Username,
		u.Email,
		u.PasswordHash,
		string(u.Role),
		u.IsVerified,
		u.CreatedAt,
		u.UpdatedAt,
	)
	if err != nil {
		// pgx expone el código de error de Postgres directamente en el mensaje;
		// usamos la constante de código 23505 (unique_violation) para no depender
		// de la lib pgerrcode que aún no agregamos al go.mod.
		if isUniqueViolation(err) {
			return user.ErrEmailAlreadyTaken
		}
		return fmt.Errorf("user_repository.Save: %w", err)
	}
	return nil
}

// ExistsByEmail returns true if a row with that email already exists.
func (r *UserRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	const q = `SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)`
	var exists bool
	if err := r.db.QueryRow(ctx, q, email).Scan(&exists); err != nil {
		return false, fmt.Errorf("user_repository.ExistsByEmail: %w", err)
	}
	return exists, nil
}

// FindByEmail retrieves a user by email. Returns user.ErrUserNotFound when missing.
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	const q = `
		SELECT id, username, email, password_hash, role, is_verified, created_at, updated_at
		FROM users WHERE email = $1`

	u, err := scanUser(r.db.QueryRow(ctx, q, email))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, user.ErrUserNotFound
		}
		return nil, fmt.Errorf("user_repository.FindByEmail: %w", err)
	}
	return u, nil
}

// FindByID retrieves a user by primary key. Returns user.ErrUserNotFound when missing.
func (r *UserRepository) FindByID(ctx context.Context, id uuid.UUID) (*user.User, error) {
	const q = `
		SELECT id, username, email, password_hash, role, is_verified, created_at, updated_at
		FROM users WHERE id = $1`

	u, err := scanUser(r.db.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, user.ErrUserNotFound
		}
		return nil, fmt.Errorf("user_repository.FindByID: %w", err)
	}
	return u, nil
}

// FindByUsername retrieves a user by username. Returns user.ErrUserNotFound when missing.
func (r *UserRepository) FindByUsername(ctx context.Context, username string) (*user.User, error) {
	const q = `
		SELECT id, username, email, password_hash, role, is_verified, created_at, updated_at
		FROM users WHERE username = $1`

	u, err := scanUser(r.db.QueryRow(ctx, q, username))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, user.ErrUserNotFound
		}
		return nil, fmt.Errorf("user_repository.FindByUsername: %w", err)
	}
	return u, nil
}

// Update persists changes to an existing user row.
func (r *UserRepository) Update(ctx context.Context, u *user.User) error {
	const q = `
		UPDATE users
		SET username = $1, email = $2, password_hash = $3,
		    role = $4, is_verified = $5, updated_at = $6
		WHERE id = $7`

	tag, err := r.db.Exec(ctx, q,
		u.Username, u.Email, u.PasswordHash,
		string(u.Role), u.IsVerified, u.UpdatedAt, u.ID,
	)
	if err != nil {
		return fmt.Errorf("user_repository.Update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return user.ErrUserNotFound
	}
	return nil
}

// ── helpers ─────────────────────────────────────────────────────────────────

// scanUser maps a single pgx row into a User aggregate.
func scanUser(row pgx.Row) (*user.User, error) {
	var u user.User
	var role string
	err := row.Scan(
		&u.ID, &u.Username, &u.Email, &u.PasswordHash,
		&role, &u.IsVerified, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	u.Role = user.Role(role)
	return &u, nil
}

// isUniqueViolation checks for Postgres error code 23505 (unique_violation).
// We inspect the error string to avoid adding pgerrcode as a dependency for now.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	// pgx wraps the PgError; its Code field is the 5-char SQLSTATE.
	type pgErr interface{ SQLState() string }
	var pe pgErr
	if errors.As(err, &pe) {
		return pe.SQLState() == "23505"
	}
	return false
}

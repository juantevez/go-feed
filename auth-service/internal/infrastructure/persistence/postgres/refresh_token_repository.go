package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	domaintoken "github.com/juantevez/my-ig/auth-service/internal/domain/token"
)

// RefreshTokenRepository persists refresh tokens for rotation and revocation.
type RefreshTokenRepository struct {
	db *pgxpool.Pool
}

func NewRefreshTokenRepository(db *pgxpool.Pool) *RefreshTokenRepository {
	return &RefreshTokenRepository{db: db}
}

// Save inserts a new refresh token record.
func (r *RefreshTokenRepository) Save(ctx context.Context, t *domaintoken.Token) error {
	const q = `
		INSERT INTO refresh_tokens (id, user_id, jti, expires_at, revoked, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := r.db.Exec(ctx, q,
		t.ID, t.UserID, t.JTI, t.ExpiresAt, t.Revoked, t.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("refresh_token_repo.Save: %w", err)
	}
	return nil
}

// FindByJTI retrieves a token by its JWT ID claim.
func (r *RefreshTokenRepository) FindByJTI(ctx context.Context, jti string) (*domaintoken.Token, error) {
	const q = `
		SELECT id, user_id, jti, expires_at, revoked, created_at
		FROM refresh_tokens WHERE jti = $1`

	var t domaintoken.Token
	err := r.db.QueryRow(ctx, q, jti).Scan(
		&t.ID, &t.UserID, &t.JTI, &t.ExpiresAt, &t.Revoked, &t.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domaintoken.ErrTokenInvalid
		}
		return nil, fmt.Errorf("refresh_token_repo.FindByJTI: %w", err)
	}
	t.Kind = domaintoken.KindRefresh
	return &t, nil
}

// Revoke marks a token as revoked by JTI.
func (r *RefreshTokenRepository) Revoke(ctx context.Context, jti string) error {
	const q = `
		UPDATE refresh_tokens
		SET revoked = TRUE, revoked_at = $1
		WHERE jti = $2 AND revoked = FALSE`

	tag, err := r.db.Exec(ctx, q, time.Now().UTC(), jti)
	if err != nil {
		return fmt.Errorf("refresh_token_repo.Revoke: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domaintoken.ErrTokenInvalid
	}
	return nil
}

// RevokeAllForUser revokes every active refresh token for a user (e.g. on logout-all).
func (r *RefreshTokenRepository) RevokeAllForUser(ctx context.Context, userID string) error {
	const q = `
		UPDATE refresh_tokens
		SET revoked = TRUE, revoked_at = $1
		WHERE user_id = $2 AND revoked = FALSE`

	_, err := r.db.Exec(ctx, q, time.Now().UTC(), userID)
	if err != nil {
		return fmt.Errorf("refresh_token_repo.RevokeAllForUser: %w", err)
	}
	return nil
}

// DeleteExpired removes tokens past their expiry — called by a background scheduler.
func (r *RefreshTokenRepository) DeleteExpired(ctx context.Context) (int64, error) {
	const q = `DELETE FROM refresh_tokens WHERE expires_at < NOW()`
	tag, err := r.db.Exec(ctx, q)
	if err != nil {
		return 0, fmt.Errorf("refresh_token_repo.DeleteExpired: %w", err)
	}
	return tag.RowsAffected(), nil
}

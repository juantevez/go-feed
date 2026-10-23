package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/juantevez/my-ig/shared/outbox"
	"github.com/juantevez/my-ig/social-service/internal/domain/follow"
)

// FollowRepository implementa follow.Repository sobre Postgres.
// Usa la tx del context si existe (outbox.Conn), así participa de TxManager.WithinTx.
type FollowRepository struct {
	db *pgxpool.Pool
}

func NewFollowRepository(db *pgxpool.Pool) *FollowRepository {
	return &FollowRepository{db: db}
}

// LockPair toma un advisory lock de transacción sobre el par sin importar el
// orden (a,b) o (b,a), así A→B y B→A simultáneos corren de a uno.
func (r *FollowRepository) LockPair(ctx context.Context, a, b uuid.UUID) error {
	lo, hi := a.String(), b.String()
	if hi < lo {
		lo, hi = hi, lo
	}
	const q = `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`
	if _, err := outbox.Conn(ctx, r.db).Exec(ctx, q, lo+":"+hi); err != nil {
		return fmt.Errorf("follow_repo.LockPair: %w", err)
	}
	return nil
}

func (r *FollowRepository) Save(ctx context.Context, f *follow.Follow) error {
	const q = `
		INSERT INTO follows (id, follower_id, following_id, mutual, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (follower_id, following_id) DO NOTHING`

	tag, err := outbox.Conn(ctx, r.db).Exec(ctx, q, f.ID, f.FollowerID, f.FollowingID, f.Mutual, f.CreatedAt)
	if err != nil {
		return fmt.Errorf("follow_repo.Save: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return follow.ErrAlreadyFollowing
	}
	return nil
}

func (r *FollowRepository) Delete(ctx context.Context, followerID, followingID uuid.UUID) error {
	const q = `DELETE FROM follows WHERE follower_id = $1 AND following_id = $2`
	tag, err := outbox.Conn(ctx, r.db).Exec(ctx, q, followerID, followingID)
	if err != nil {
		return fmt.Errorf("follow_repo.Delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return follow.ErrNotFollowing
	}
	return nil
}

func (r *FollowRepository) Exists(ctx context.Context, followerID, followingID uuid.UUID) (bool, error) {
	const q = `SELECT EXISTS(SELECT 1 FROM follows WHERE follower_id = $1 AND following_id = $2)`
	var exists bool
	if err := outbox.Conn(ctx, r.db).QueryRow(ctx, q, followerID, followingID).Scan(&exists); err != nil {
		return false, fmt.Errorf("follow_repo.Exists: %w", err)
	}
	return exists, nil
}

func (r *FollowRepository) GetFollowers(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*follow.Follow, error) {
	const q = `
		SELECT id, follower_id, following_id, mutual, created_at
		FROM follows
		WHERE following_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	return r.queryFollows(ctx, q, userID, limit, offset)
}

func (r *FollowRepository) GetFollowing(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*follow.Follow, error) {
	const q = `
		SELECT id, follower_id, following_id, mutual, created_at
		FROM follows
		WHERE follower_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	return r.queryFollows(ctx, q, userID, limit, offset)
}

func (r *FollowRepository) GetStats(ctx context.Context, userID uuid.UUID) (*follow.Stats, error) {
	const q = `
		SELECT
			(SELECT COUNT(*) FROM follows WHERE following_id = $1) AS follower_count,
			(SELECT COUNT(*) FROM follows WHERE follower_id  = $1) AS following_count`

	var stats follow.Stats
	stats.UserID = userID
	err := outbox.Conn(ctx, r.db).QueryRow(ctx, q, userID).Scan(&stats.FollowerCount, &stats.FollowingCount)
	if err != nil {
		return nil, fmt.Errorf("follow_repo.GetStats: %w", err)
	}
	return &stats, nil
}

// UpdateMutual actualiza las dos direcciones del par. Antes tocaba solo
// followerID → followingID, y la fila del otro lado quedaba desactualizada.
func (r *FollowRepository) UpdateMutual(ctx context.Context, followerID, followingID uuid.UUID, mutual bool) error {
	const q = `
		UPDATE follows SET mutual = $1
		WHERE (follower_id = $2 AND following_id = $3)
		   OR (follower_id = $3 AND following_id = $2)`
	_, err := outbox.Conn(ctx, r.db).Exec(ctx, q, mutual, followerID, followingID)
	if err != nil {
		return fmt.Errorf("follow_repo.UpdateMutual: %w", err)
	}
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (r *FollowRepository) queryFollows(ctx context.Context, q string, userID uuid.UUID, limit, offset int) ([]*follow.Follow, error) {
	rows, err := outbox.Conn(ctx, r.db).Query(ctx, q, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("follow_repo query: %w", err)
	}
	defer rows.Close()

	var follows []*follow.Follow
	for rows.Next() {
		f, err := scanFollow(rows)
		if err != nil {
			return nil, fmt.Errorf("follow_repo scan: %w", err)
		}
		follows = append(follows, f)
	}
	return follows, rows.Err()
}

func scanFollow(row pgx.Row) (*follow.Follow, error) {
	var f follow.Follow
	err := row.Scan(&f.ID, &f.FollowerID, &f.FollowingID, &f.Mutual, &f.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, follow.ErrFollowNotFound
		}
		return nil, err
	}
	return &f, nil
}

package input

import (
	"context"

	"github.com/google/uuid"
	"github.com/juantevez/my-ig/social-service/internal/domain/follow"
)

// ── Follow ────────────────────────────────────────────────────────────────────

type FollowCommand struct {
	FollowerID  uuid.UUID
	FollowingID uuid.UUID
	TraceID     string
}

// ── Unfollow ──────────────────────────────────────────────────────────────────

type UnfollowCommand struct {
	FollowerID  uuid.UUID
	FollowingID uuid.UUID
	TraceID     string
}

// ── GetFollowers / GetFollowing ───────────────────────────────────────────────

type GetFollowersQuery struct {
	UserID uuid.UUID
	Limit  int
	Offset int
}

type GetFollowingQuery struct {
	UserID uuid.UUID
	Limit  int
	Offset int
}

type FollowListResult struct {
	Follows []*follow.Follow
	Total   int
}

// ── FollowUseCase ─────────────────────────────────────────────────────────────

// FollowUseCase es el puerto driving del social-service.
type FollowUseCase interface {
	Follow(ctx context.Context, cmd FollowCommand) error
	Unfollow(ctx context.Context, cmd UnfollowCommand) error
	GetFollowers(ctx context.Context, q GetFollowersQuery) (*FollowListResult, error)
	GetFollowing(ctx context.Context, q GetFollowingQuery) (*FollowListResult, error)
	GetStats(ctx context.Context, userID uuid.UUID) (*follow.Stats, error)
}

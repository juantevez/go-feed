package events

import (
	"time"

	"github.com/google/uuid"
)

// ── users.followed.v1 ─────────────────────────────────────────────────────────

// UserFollowedPayload es el payload del evento TopicUserFollowed.
// Producer: social-service
// Consumers: feed-service (rebuild parcial del feed), notification-service
type UserFollowedPayload struct {
	FollowerID  uuid.UUID `json:"follower_id"`  // quien sigue
	FollowingID uuid.UUID `json:"following_id"` // a quien sigue
	Mutual      bool      `json:"mutual"`       // true si ambos se siguen
	FollowedAt  time.Time `json:"followed_at"`
}

// ── users.unfollowed.v1 ───────────────────────────────────────────────────────

// UserUnfollowedPayload es el payload del evento TopicUserUnfollowed.
// Producer: social-service
// Consumers: feed-service (ZREM posts del autor del feed del follower)
type UserUnfollowedPayload struct {
	FollowerID   uuid.UUID `json:"follower_id"`
	FollowingID  uuid.UUID `json:"following_id"`
	UnfollowedAt time.Time `json:"unfollowed_at"`
}

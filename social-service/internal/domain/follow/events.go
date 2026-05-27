package follow

import (
	"time"

	"github.com/google/uuid"
	"github.com/juantevez/my-ig/shared/events"
)

// NewFollowedEnvelope construye el Envelope para users.followed.v1.
func NewFollowedEnvelope(f *Follow, traceID string) events.Envelope {
	payload := events.UserFollowedPayload{
		FollowerID:  f.FollowerID,
		FollowingID: f.FollowingID,
		Mutual:      f.Mutual,
		FollowedAt:  f.CreatedAt,
	}
	return events.Wrap(
		events.TopicUserFollowed,
		"social-service",
		idempotencyKey("follow", f.FollowerID, f.FollowingID),
		traceID,
		payload,
	)
}

// NewUnfollowedEnvelope construye el Envelope para users.unfollowed.v1.
func NewUnfollowedEnvelope(followerID, followingID uuid.UUID, traceID string) events.Envelope {
	payload := events.UserUnfollowedPayload{
		FollowerID:   followerID,
		FollowingID:  followingID,
		UnfollowedAt: time.Now().UTC(),
	}
	return events.Wrap(
		events.TopicUserUnfollowed,
		"social-service",
		idempotencyKey("unfollow", followerID, followingID),
		traceID,
		payload,
	)
}

func idempotencyKey(action string, a, b uuid.UUID) string {
	return "idemp:social:" + action + ":" + a.String() + ":" + b.String()
}

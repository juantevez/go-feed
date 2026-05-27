package events

import (
	"time"

	"github.com/google/uuid"
)

// ── posts.created.v1 ──────────────────────────────────────────────────────────

// PostCreatedPayload es el payload del evento TopicPostCreated.
// Producer: content-service | Consumers: feed-service, notification-service
type PostCreatedPayload struct {
	PostID     uuid.UUID `json:"post_id"`
	AuthorID   uuid.UUID `json:"author_id"`
	MediaCount int       `json:"media_count"`
	Visibility string    `json:"visibility"` // "public", "followers_only", "private"
	Tags       []string  `json:"tags,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// ── posts.deleted.v1 ──────────────────────────────────────────────────────────

// PostDeletedPayload es el payload del evento TopicPostDeleted.
// Producer: content-service | Consumers: feed-service (invalidar caché)
type PostDeletedPayload struct {
	PostID    uuid.UUID `json:"post_id"`
	AuthorID  uuid.UUID `json:"author_id"`
	Reason    string    `json:"reason,omitempty"` // "user_request", "moderation", "expired"
	DeletedAt time.Time `json:"deleted_at"`
}

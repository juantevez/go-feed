package post

import (
	"time"

	"github.com/google/uuid"
	"github.com/juantevez/my-ig/shared/events"
)

// NewCreatedEnvelope construye el Envelope para posts.created.v1.
func NewCreatedEnvelope(p *Post, traceID string) events.Envelope {
	payload := events.PostCreatedPayload{
		PostID:     p.ID,
		AuthorID:   p.AuthorID,
		MediaCount: len(p.MediaURLs),
		Visibility: string(p.Visibility),
		Tags:       p.Tags,
		CreatedAt:  p.CreatedAt,
	}
	return events.Wrap(
		events.TopicPostCreated,
		"content-service",
		idempotencyKey("create", p.ID),
		traceID,
		payload,
	)
}

// NewDeletedEnvelope construye el Envelope para posts.deleted.v1.
func NewDeletedEnvelope(p *Post, reason, traceID string) events.Envelope {
	payload := events.PostDeletedPayload{
		PostID:    p.ID,
		AuthorID:  p.AuthorID,
		Reason:    reason,
		DeletedAt: time.Now().UTC(),
	}
	return events.Wrap(
		events.TopicPostDeleted,
		"content-service",
		idempotencyKey("delete", p.ID),
		traceID,
		payload,
	)
}

func idempotencyKey(action string, postID uuid.UUID) string {
	return "idemp:content:" + action + ":" + postID.String()
}

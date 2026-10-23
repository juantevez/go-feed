package natsconsumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"

	"github.com/juantevez/my-ig/feed-service/internal/application/port/input"
	"github.com/juantevez/my-ig/shared/dlq"
	"github.com/juantevez/my-ig/shared/events"
)

// Consumer suscribe al bus NATS JetStream y despacha eventos
// a los use cases correspondientes.
type Consumer struct {
	js           nats.JetStreamContext
	fanOut       input.FanOutUseCase
	feedRepo     feedDeleter
	followerRepo followerWriter
}

// feedDeleter es la interfaz mínima que el consumer necesita para invalidación.
type feedDeleter interface {
	DeleteByPostID(ctx context.Context, postID uuid.UUID) error
}

// followerWriter sincroniza el grafo social en la DB local del feed-service.
// Ambos métodos son last-write-wins por timestamp del evento: followed y
// unfollowed llegan por subscriptions paralelas y pueden procesarse desordenados.
// Retornan false si el evento era más viejo que el estado actual (se descarta).
type followerWriter interface {
	SaveFollower(ctx context.Context, followerID, followingID uuid.UUID, at time.Time) (bool, error)
	MarkUnfollowed(ctx context.Context, followerID, followingID uuid.UUID, at time.Time) (bool, error)
}

func NewConsumer(js nats.JetStreamContext, fanOut input.FanOutUseCase, feedRepo feedDeleter, followerRepo followerWriter) *Consumer {
	return &Consumer{js: js, fanOut: fanOut, feedRepo: feedRepo, followerRepo: followerRepo}
}

// Subscribe registra todos los consumers JetStream del feed-service.
// Debe llamarse una sola vez al arrancar el servicio.
//
// Los handlers retornan error en vez de hacer Ack/Nak: dlq.Subscribe aplica
// reintentos con backoff y, si se agotan (o el error es dlq.Permanent), manda
// el mensaje al stream DLQ.
func (c *Consumer) Subscribe() error {
	if err := dlq.EnsureStream(c.js); err != nil {
		return err
	}

	subs := []struct {
		topic   string
		durable string
		handler dlq.Handler
	}{
		{events.TopicPostCreated, "feed-service-post-created", c.handlePostCreated},
		{events.TopicPostDeleted, "feed-service-post-deleted", c.handlePostDeleted},
		{events.TopicUserFollowed, "feed-service-user-followed", c.handleUserFollowed},
		{events.TopicUserUnfollowed, "feed-service-user-unfollowed", c.handleUserUnfollowed},
	}
	for _, sub := range subs {
		if err := dlq.Subscribe(c.js, sub.topic, sub.durable, sub.handler); err != nil {
			return fmt.Errorf("consumer: %w", err)
		}
		slog.Info("consumer: subscribed", "topic", sub.topic)
	}
	return nil
}

// ── posts.created.v1 ──────────────────────────────────────────────────────────

func (c *Consumer) handlePostCreated(ctx context.Context, msg *nats.Msg) error {
	var envelope events.Envelope
	if err := json.Unmarshal(msg.Data, &envelope); err != nil {
		// Payload malformado: reintentar no lo va a arreglar.
		return dlq.Permanent(fmt.Errorf("unmarshal envelope: %w", err))
	}

	// El payload viene como map[string]any por el any del Envelope.
	// Lo re-serializamos al tipo concreto.
	cmd, err := extractPostCreatedCommand(envelope)
	if err != nil {
		return dlq.Permanent(fmt.Errorf("extract post created payload (event_id=%s): %w", envelope.EventID, err))
	}

	slog.Info("consumer: processing post created",
		"event_id", envelope.EventID,
		"post_id", cmd.PostID,
		"author_id", cmd.AuthorID,
	)

	if err := c.fanOut.FanOut(ctx, cmd); err != nil {
		return fmt.Errorf("fan out (post_id=%s): %w", cmd.PostID, err)
	}
	return nil
}

// ── posts.deleted.v1 ──────────────────────────────────────────────────────────

func (c *Consumer) handlePostDeleted(ctx context.Context, msg *nats.Msg) error {
	var envelope events.Envelope
	if err := json.Unmarshal(msg.Data, &envelope); err != nil {
		return dlq.Permanent(fmt.Errorf("unmarshal envelope: %w", err))
	}

	postID, err := extractPostID(envelope)
	if err != nil {
		return dlq.Permanent(fmt.Errorf("extract post deleted payload (event_id=%s): %w", envelope.EventID, err))
	}

	slog.Info("consumer: invalidating feed for deleted post", "post_id", postID)

	if err := c.feedRepo.DeleteByPostID(ctx, postID); err != nil {
		return fmt.Errorf("delete feed entries (post_id=%s): %w", postID, err)
	}
	return nil
}

// ── users.followed.v1 ─────────────────────────────────────────────────────────

func (c *Consumer) handleUserFollowed(ctx context.Context, msg *nats.Msg) error {
	var envelope events.Envelope
	if err := json.Unmarshal(msg.Data, &envelope); err != nil {
		return dlq.Permanent(fmt.Errorf("unmarshal envelope: %w", err))
	}

	var p events.UserFollowedPayload
	if err := decodePayload(envelope, &p); err != nil {
		return dlq.Permanent(fmt.Errorf("extract user followed payload (event_id=%s): %w", envelope.EventID, err))
	}

	slog.Info("consumer: saving follower", "follower_id", p.FollowerID, "following_id", p.FollowingID)

	applied, err := c.followerRepo.SaveFollower(ctx, p.FollowerID, p.FollowingID, p.FollowedAt)
	if err != nil {
		return fmt.Errorf("save follower: %w", err)
	}
	if !applied {
		slog.Info("consumer: stale follow event ignored", "event_id", envelope.EventID,
			"follower_id", p.FollowerID, "following_id", p.FollowingID)
	}
	return nil
}

// ── users.unfollowed.v1 ───────────────────────────────────────────────────────

func (c *Consumer) handleUserUnfollowed(ctx context.Context, msg *nats.Msg) error {
	var envelope events.Envelope
	if err := json.Unmarshal(msg.Data, &envelope); err != nil {
		return dlq.Permanent(fmt.Errorf("unmarshal envelope: %w", err))
	}

	var p events.UserUnfollowedPayload
	if err := decodePayload(envelope, &p); err != nil {
		return dlq.Permanent(fmt.Errorf("extract user unfollowed payload (event_id=%s): %w", envelope.EventID, err))
	}

	slog.Info("consumer: removing follower", "follower_id", p.FollowerID, "following_id", p.FollowingID)

	applied, err := c.followerRepo.MarkUnfollowed(ctx, p.FollowerID, p.FollowingID, p.UnfollowedAt)
	if err != nil {
		return fmt.Errorf("mark unfollowed: %w", err)
	}
	if !applied {
		slog.Info("consumer: stale unfollow event ignored", "event_id", envelope.EventID,
			"follower_id", p.FollowerID, "following_id", p.FollowingID)
	}
	return nil
}

// ── helpers de extracción de payload ─────────────────────────────────────────

// decodePayload re-serializa el payload any del Envelope al tipo concreto.
func decodePayload(env events.Envelope, dst any) error {
	raw, err := json.Marshal(env.Payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}

// extractPostCreatedCommand convierte el payload de posts.created.v1 al comando del use case.
func extractPostCreatedCommand(env events.Envelope) (input.FanOutCommand, error) {
	var p events.PostCreatedPayload
	if err := decodePayload(env, &p); err != nil {
		return input.FanOutCommand{}, err
	}
	return input.FanOutCommand{
		PostID:      p.PostID,
		AuthorID:    p.AuthorID,
		PublishedAt: p.CreatedAt,
	}, nil
}

func extractPostID(env events.Envelope) (uuid.UUID, error) {
	var p events.PostDeletedPayload
	if err := decodePayload(env, &p); err != nil {
		return uuid.Nil, err
	}
	return p.PostID, nil
}

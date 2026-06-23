package natsconsumer

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"

	"github.com/juantevez/my-ig/feed-service/internal/application/port/input"
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
type followerWriter interface {
	SaveFollower(ctx context.Context, followerID, followingID uuid.UUID) error
	DeleteFollower(ctx context.Context, followerID, followingID uuid.UUID) error
}

func NewConsumer(js nats.JetStreamContext, fanOut input.FanOutUseCase, feedRepo feedDeleter, followerRepo followerWriter) *Consumer {
	return &Consumer{js: js, fanOut: fanOut, feedRepo: feedRepo, followerRepo: followerRepo}
}

// Subscribe registra todos los consumers JetStream del feed-service.
// Debe llamarse una sola vez al arrancar el servicio.
func (c *Consumer) Subscribe() error {
	if err := c.subscribePostCreated(); err != nil {
		return err
	}
	if err := c.subscribePostDeleted(); err != nil {
		return err
	}
	if err := c.subscribeUserFollowed(); err != nil {
		return err
	}
	if err := c.subscribeUserUnfollowed(); err != nil {
		return err
	}
	return nil
}

// ── posts.created.v1 ──────────────────────────────────────────────────────────

func (c *Consumer) subscribePostCreated() error {
	_, err := c.js.Subscribe(
		events.TopicPostCreated,
		c.handlePostCreated,
		nats.Durable("feed-service-post-created"),
		nats.AckExplicit(),
		nats.MaxDeliver(3),           // máximo 3 reintentos antes de DLQ
		nats.AckWait(30*time.Second), // timeout por mensaje
		nats.DeliverNew(),            // solo mensajes nuevos al arrancar
	)
	if err != nil {
		return errors.New("consumer: subscribe posts.created.v1: " + err.Error())
	}
	slog.Info("consumer: subscribed", "topic", events.TopicPostCreated)
	return nil
}

func (c *Consumer) handlePostCreated(msg *nats.Msg) {
	ctx := context.Background()

	var envelope events.Envelope
	if err := json.Unmarshal(msg.Data, &envelope); err != nil {
		slog.Error("consumer: unmarshal envelope",
			"topic", events.TopicPostCreated,
			"err", err,
		)
		// NAK sin reintento — payload malformado no se va a recuperar.
		_ = msg.Term()
		return
	}

	// El payload viene como map[string]any por el any del Envelope.
	// Lo re-serializamos al tipo concreto.
	cmd, err := extractPostCreatedCommand(envelope)
	if err != nil {
		slog.Error("consumer: extract post created payload",
			"event_id", envelope.EventID,
			"err", err,
		)
		_ = msg.Term()
		return
	}

	slog.Info("consumer: processing post created",
		"event_id", envelope.EventID,
		"post_id", cmd.PostID,
		"author_id", cmd.AuthorID,
	)

	if err := c.fanOut.FanOut(ctx, cmd); err != nil {
		slog.Error("consumer: fan out failed",
			"event_id", envelope.EventID,
			"post_id", cmd.PostID,
			"err", err,
		)
		// NAK — JetStream reintentará según MaxDeliver.
		_ = msg.Nak()
		return
	}

	_ = msg.Ack()
}

// ── posts.deleted.v1 ──────────────────────────────────────────────────────────

func (c *Consumer) subscribePostDeleted() error {
	_, err := c.js.Subscribe(
		events.TopicPostDeleted,
		c.handlePostDeleted,
		nats.Durable("feed-service-post-deleted"),
		nats.AckExplicit(),
		nats.MaxDeliver(3),
		nats.AckWait(30*time.Second),
		nats.DeliverNew(),
	)
	if err != nil {
		return errors.New("consumer: subscribe posts.deleted.v1: " + err.Error())
	}
	slog.Info("consumer: subscribed", "topic", events.TopicPostDeleted)
	return nil
}

func (c *Consumer) handlePostDeleted(msg *nats.Msg) {
	ctx := context.Background()

	var envelope events.Envelope
	if err := json.Unmarshal(msg.Data, &envelope); err != nil {
		slog.Error("consumer: unmarshal envelope", "topic", events.TopicPostDeleted, "err", err)
		_ = msg.Term()
		return
	}

	postID, err := extractPostID(envelope)
	if err != nil {
		slog.Error("consumer: extract post deleted payload", "event_id", envelope.EventID, "err", err)
		_ = msg.Term()
		return
	}

	slog.Info("consumer: invalidating feed for deleted post", "post_id", postID)

	if err := c.feedRepo.DeleteByPostID(ctx, postID); err != nil {
		slog.Error("consumer: delete feed entries", "post_id", postID, "err", err)
		_ = msg.Nak()
		return
	}

	_ = msg.Ack()
}

// ── helpers de extracción de payload ─────────────────────────────────────────

// extractPostCreatedCommand re-serializa el payload any del Envelope
// al tipo concreto PostCreatedPayload y lo convierte al comando del use case.
func extractPostCreatedCommand(env events.Envelope) (input.FanOutCommand, error) {
	raw, err := json.Marshal(env.Payload)
	if err != nil {
		return input.FanOutCommand{}, err
	}

	var p events.PostCreatedPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return input.FanOutCommand{}, err
	}

	return input.FanOutCommand{
		PostID:      p.PostID,
		AuthorID:    p.AuthorID,
		PublishedAt: p.CreatedAt,
	}, nil
}

func extractPostID(env events.Envelope) (uuid.UUID, error) {
	raw, err := json.Marshal(env.Payload)
	if err != nil {
		return uuid.Nil, err
	}

	var p events.PostDeletedPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return uuid.Nil, err
	}

	return p.PostID, nil
}

// ── users.followed.v1 ─────────────────────────────────────────────────────────

func (c *Consumer) subscribeUserFollowed() error {
	_, err := c.js.Subscribe(
		events.TopicUserFollowed,
		c.handleUserFollowed,
		nats.Durable("feed-service-user-followed"),
		nats.AckExplicit(),
		nats.MaxDeliver(3),
		nats.AckWait(30*time.Second),
		nats.DeliverNew(),
	)
	if err != nil {
		return errors.New("consumer: subscribe users.followed.v1: " + err.Error())
	}
	slog.Info("consumer: subscribed", "topic", events.TopicUserFollowed)
	return nil
}

func (c *Consumer) handleUserFollowed(msg *nats.Msg) {
	ctx := context.Background()

	var envelope events.Envelope
	if err := json.Unmarshal(msg.Data, &envelope); err != nil {
		slog.Error("consumer: unmarshal envelope", "topic", events.TopicUserFollowed, "err", err)
		_ = msg.Term()
		return
	}

	raw, err := json.Marshal(envelope.Payload)
	if err != nil {
		_ = msg.Term()
		return
	}
	var p events.UserFollowedPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		slog.Error("consumer: extract user followed payload", "event_id", envelope.EventID, "err", err)
		_ = msg.Term()
		return
	}

	slog.Info("consumer: saving follower", "follower_id", p.FollowerID, "following_id", p.FollowingID)

	if err := c.followerRepo.SaveFollower(ctx, p.FollowerID, p.FollowingID); err != nil {
		slog.Error("consumer: save follower failed", "err", err)
		_ = msg.Nak()
		return
	}

	_ = msg.Ack()
}

// ── users.unfollowed.v1 ───────────────────────────────────────────────────────

func (c *Consumer) subscribeUserUnfollowed() error {
	_, err := c.js.Subscribe(
		events.TopicUserUnfollowed,
		c.handleUserUnfollowed,
		nats.Durable("feed-service-user-unfollowed"),
		nats.AckExplicit(),
		nats.MaxDeliver(3),
		nats.AckWait(30*time.Second),
		nats.DeliverNew(),
	)
	if err != nil {
		return errors.New("consumer: subscribe users.unfollowed.v1: " + err.Error())
	}
	slog.Info("consumer: subscribed", "topic", events.TopicUserUnfollowed)
	return nil
}

func (c *Consumer) handleUserUnfollowed(msg *nats.Msg) {
	ctx := context.Background()

	var envelope events.Envelope
	if err := json.Unmarshal(msg.Data, &envelope); err != nil {
		slog.Error("consumer: unmarshal envelope", "topic", events.TopicUserUnfollowed, "err", err)
		_ = msg.Term()
		return
	}

	raw, err := json.Marshal(envelope.Payload)
	if err != nil {
		_ = msg.Term()
		return
	}
	var p events.UserUnfollowedPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		slog.Error("consumer: extract user unfollowed payload", "event_id", envelope.EventID, "err", err)
		_ = msg.Term()
		return
	}

	slog.Info("consumer: removing follower", "follower_id", p.FollowerID, "following_id", p.FollowingID)

	if err := c.followerRepo.DeleteFollower(ctx, p.FollowerID, p.FollowingID); err != nil {
		slog.Error("consumer: delete follower failed", "err", err)
		_ = msg.Nak()
		return
	}

	_ = msg.Ack()
}

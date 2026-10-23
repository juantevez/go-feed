package natsconsumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/juantevez/my-ig/notification-service/internal/application/usecase"
	"github.com/juantevez/my-ig/notification-service/internal/domain/notification"
	"github.com/juantevez/my-ig/shared/dlq"
	"github.com/juantevez/my-ig/shared/events"
	"github.com/nats-io/nats.go"
)

// Consumer suscribe al bus NATS y genera notificaciones por cada evento relevante.
type Consumer struct {
	js  nats.JetStreamContext
	svc *usecase.NotificationService
}

func NewConsumer(js nats.JetStreamContext, svc *usecase.NotificationService) *Consumer {
	return &Consumer{js: js, svc: svc}
}

// Subscribe registra todos los consumers del notification-service.
// Reintentos y DLQ los maneja dlq.Subscribe: los handlers solo retornan error.
func (c *Consumer) Subscribe() error {
	if err := dlq.EnsureStream(c.js); err != nil {
		return err
	}

	subs := []struct {
		topic   string
		durable string
		handler dlq.Handler
	}{
		{events.TopicUserFollowed, "notification-service-user-followed", c.handleUserFollowed},
		{events.TopicPostCreated, "notification-service-post-created", c.handlePostCreated},
	}
	for _, sub := range subs {
		if err := dlq.Subscribe(c.js, sub.topic, sub.durable, sub.handler); err != nil {
			return fmt.Errorf("consumer: %w", err)
		}
		slog.Info("consumer: subscribed", "topic", sub.topic)
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
		return dlq.Permanent(fmt.Errorf("extract payload (event_id=%s): %w", envelope.EventID, err))
	}

	n, err := notification.New(
		p.FollowingID, // quien recibe: el seguido
		p.FollowerID,  // quien genera: el seguidor
		notification.TypeNewFollower,
		map[string]string{"follower_id": p.FollowerID.String()},
	)
	if err != nil {
		return dlq.Permanent(fmt.Errorf("build notification (event_id=%s): %w", envelope.EventID, err))
	}

	if err := c.svc.CreateAndPush(ctx, n.FromEvent(envelope.EventID)); err != nil {
		return fmt.Errorf("create and push: %w", err)
	}
	return nil
}

// ── posts.created.v1 ──────────────────────────────────────────────────────────

func (c *Consumer) handlePostCreated(_ context.Context, msg *nats.Msg) error {
	var envelope events.Envelope
	if err := json.Unmarshal(msg.Data, &envelope); err != nil {
		return dlq.Permanent(fmt.Errorf("unmarshal envelope: %w", err))
	}

	var p events.PostCreatedPayload
	if err := decodePayload(envelope, &p); err != nil {
		return dlq.Permanent(fmt.Errorf("extract payload (event_id=%s): %w", envelope.EventID, err))
	}

	// TODO fase 2: notificar a seguidores del autor.
	// Por ahora solo logueamos — requiere consultar el grafo social.
	slog.Info("consumer: post created, notifications pending",
		"post_id", p.PostID,
		"author_id", p.AuthorID,
	)
	return nil
}

// decodePayload re-serializa el payload any del Envelope al tipo concreto.
func decodePayload(env events.Envelope, dst any) error {
	raw, err := json.Marshal(env.Payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}

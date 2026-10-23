package natsconsumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

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
	if err := ensureStreams(c.js); err != nil {
		return err
	}

	subs := []struct {
		topic   string
		durable string
		handler dlq.Handler
	}{
		{events.TopicUserFollowed, "notification-service-user-followed", c.handleUserFollowed},
		{events.TopicUserUnfollowed, "notification-service-user-unfollowed", c.handleUserUnfollowed},
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

	if err := c.svc.NotifyNewFollower(ctx, n.FromEvent(envelope.EventID), p.FollowedAt); err != nil {
		return err
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
		return dlq.Permanent(fmt.Errorf("extract payload (event_id=%s): %w", envelope.EventID, err))
	}

	// Dejar de seguir borra la notificación "X te siguió".
	return c.svc.RemoveNewFollower(ctx, p.FollowerID, p.FollowingID, p.UnfollowedAt)
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

// ensureStreams crea los streams de los que consume este servicio, si no existen.
// Sin esto, arrancar antes que content-service o feed-service sobre un NATS vacío
// hacía fallar la suscripción. Misma configuración que en esos servicios.
func ensureStreams(js nats.JetStreamContext) error {
	streams := []struct {
		name     string
		subjects []string
	}{
		{"POSTS", []string{"posts.>"}},
		{"USERS", []string{"users.>"}},
	}
	for _, s := range streams {
		_, err := js.AddStream(&nats.StreamConfig{
			Name:     s.name,
			Subjects: s.subjects,
			Storage:  nats.FileStorage,
			MaxAge:   7 * 24 * time.Hour,
		})
		if err != nil && !errors.Is(err, nats.ErrStreamNameAlreadyInUse) {
			return fmt.Errorf("ensure stream %s: %w", s.name, err)
		}
	}
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

package natsconsumer

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/juantevez/my-ig/notification-service/internal/application/usecase"
	"github.com/juantevez/my-ig/notification-service/internal/domain/notification"
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
func (c *Consumer) Subscribe() error {
	if err := c.subscribeUserFollowed(); err != nil {
		return err
	}
	if err := c.subscribePostCreated(); err != nil {
		return err
	}
	return nil
}

// ── users.followed.v1 ─────────────────────────────────────────────────────────

func (c *Consumer) subscribeUserFollowed() error {
	_, err := c.js.Subscribe(
		events.TopicUserFollowed,
		c.handleUserFollowed,
		nats.Durable("notification-service-user-followed"),
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
	var envelope events.Envelope
	if err := json.Unmarshal(msg.Data, &envelope); err != nil {
		slog.Error("consumer: unmarshal", "topic", events.TopicUserFollowed, "err", err)
		_ = msg.Term()
		return
	}

	raw, _ := json.Marshal(envelope.Payload)
	var p events.UserFollowedPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		slog.Error("consumer: extract payload", "event_id", envelope.EventID, "err", err)
		_ = msg.Term()
		return
	}

	n, err := notification.New(
		p.FollowingID, // quien recibe: el seguido
		p.FollowerID,  // quien genera: el seguidor
		notification.TypeNewFollower,
		map[string]string{"follower_id": p.FollowerID.String()},
	)
	if err != nil {
		slog.Error("consumer: build notification", "err", err)
		_ = msg.Term()
		return
	}

	if err := c.svc.CreateAndPush(context.Background(), n); err != nil {
		slog.Error("consumer: create and push", "err", err)
		_ = msg.Nak()
		return
	}

	_ = msg.Ack()
}

// ── posts.created.v1 ──────────────────────────────────────────────────────────

func (c *Consumer) subscribePostCreated() error {
	_, err := c.js.Subscribe(
		events.TopicPostCreated,
		c.handlePostCreated,
		nats.Durable("notification-service-post-created"),
		nats.AckExplicit(),
		nats.MaxDeliver(3),
		nats.AckWait(30*time.Second),
		nats.DeliverNew(),
	)
	if err != nil {
		return errors.New("consumer: subscribe posts.created.v1: " + err.Error())
	}
	slog.Info("consumer: subscribed", "topic", events.TopicPostCreated)
	return nil
}

func (c *Consumer) handlePostCreated(msg *nats.Msg) {
	var envelope events.Envelope
	if err := json.Unmarshal(msg.Data, &envelope); err != nil {
		slog.Error("consumer: unmarshal", "topic", events.TopicPostCreated, "err", err)
		_ = msg.Term()
		return
	}

	raw, _ := json.Marshal(envelope.Payload)
	var p events.PostCreatedPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		slog.Error("consumer: extract payload", "event_id", envelope.EventID, "err", err)
		_ = msg.Term()
		return
	}

	// TODO fase 2: notificar a seguidores del autor.
	// Por ahora solo logueamos — requiere consultar el grafo social.
	slog.Info("consumer: post created, notifications pending",
		"post_id", p.PostID,
		"author_id", p.AuthorID,
	)

	_ = msg.Ack()
}

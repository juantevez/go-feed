package natsconsumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"

	"github.com/juantevez/my-ig/shared/dlq"
	"github.com/juantevez/my-ig/shared/events"
)

// userRegistrar es lo que el consumer necesita del caso de uso.
type userRegistrar interface {
	RegisterUser(ctx context.Context, id uuid.UUID, username string, registeredAt time.Time) error
}

// Consumer suscribe social-service a los eventos que le interesan.
// Reintentos y DLQ los maneja dlq.Subscribe: los handlers solo retornan error.
type Consumer struct {
	js    nats.JetStreamContext
	users userRegistrar
}

func NewConsumer(js nats.JetStreamContext, users userRegistrar) *Consumer {
	return &Consumer{js: js, users: users}
}

// Subscribe registra los consumers. Debe llamarse una sola vez al arrancar.
func (c *Consumer) Subscribe() error {
	if err := dlq.EnsureStream(c.js); err != nil {
		return err
	}
	const durable = "social-service-user-registered"
	if err := dlq.Subscribe(c.js, events.TopicAuthUserRegistered, durable, c.handleUserRegistered); err != nil {
		return fmt.Errorf("consumer: %w", err)
	}
	slog.Info("consumer: subscribed", "topic", events.TopicAuthUserRegistered)
	return nil
}

// ── auth.user.registered.v1 ───────────────────────────────────────────────────

func (c *Consumer) handleUserRegistered(ctx context.Context, msg *nats.Msg) error {
	var envelope events.Envelope
	if err := json.Unmarshal(msg.Data, &envelope); err != nil {
		return dlq.Permanent(fmt.Errorf("unmarshal envelope: %w", err))
	}

	var p events.UserRegisteredPayload
	if err := decodePayload(envelope, &p); err != nil {
		return dlq.Permanent(fmt.Errorf("extract payload (event_id=%s): %w", envelope.EventID, err))
	}

	if err := c.users.RegisterUser(ctx, p.UserID, p.Username, p.OccuredAt); err != nil {
		return fmt.Errorf("register user (user_id=%s): %w", p.UserID, err)
	}
	slog.InfoContext(ctx, "consumer: user registered", "user_id", p.UserID)
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

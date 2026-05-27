package nats

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
)

// Publisher is the NATS JetStream adapter for output.EventPublisher.
type Publisher struct {
	js nats.JetStreamContext
}

func NewPublisher(js nats.JetStreamContext) *Publisher {
	return &Publisher{js: js}
}

// Publish serialises the payload to JSON and publishes it to the given topic.
// Topic convention: domain.entity.action.version → e.g. "auth.user.registered.v1"
func (p *Publisher) Publish(ctx context.Context, topic string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("nats publisher: marshal payload: %w", err)
	}

	// TODO: add envelope (event_id, trace_id, timestamp) as per contract before publishing.
	if _, err := p.js.Publish(topic, data); err != nil {
		return fmt.Errorf("nats publisher: publish to %s: %w", topic, err)
	}
	return nil
}

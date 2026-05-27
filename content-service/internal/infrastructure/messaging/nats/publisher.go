package natspublisher

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/juantevez/my-ig/shared/events"
	"github.com/nats-io/nats.go"
)

// Publisher implementa output.EventPublisher sobre NATS JetStream.
type Publisher struct {
	js nats.JetStreamContext
}

func New(js nats.JetStreamContext) *Publisher {
	return &Publisher{js: js}
}

func (p *Publisher) Publish(_ context.Context, topic string, payload any) error {
	var envelope events.Envelope
	switch v := payload.(type) {
	case events.Envelope:
		envelope = v
	default:
		envelope = events.Wrap(topic, "content-service", "", "", payload)
	}

	data, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("nats publisher: marshal: %w", err)
	}
	if _, err := p.js.Publish(topic, data); err != nil {
		return fmt.Errorf("nats publisher: publish to %s: %w", topic, err)
	}
	return nil
}

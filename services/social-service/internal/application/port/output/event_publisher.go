package output

import "context"

// EventPublisher es el puerto driven para publicar eventos al bus.
type EventPublisher interface {
	Publish(ctx context.Context, topic string, payload any) error
}

package output

import "context"

// EventPublisher is the driven port for async event emission.
// The NATS adapter implements this; tests can use a no-op or spy.
type EventPublisher interface {
	// Publish sends a domain event to the bus.
	// topic follows the convention: domain.entity.action.version (e.g. "auth.user.registered.v1")
	Publish(ctx context.Context, topic string, payload any) error
}

package output

import "context"

// EventPublisher es el puerto driven para publicar eventos al bus.
// Misma interfaz que en auth-service — compartir la interfaz en shared
// sería over-engineering; cada servicio define la suya.
type EventPublisher interface {
	Publish(ctx context.Context, topic string, payload any) error
}

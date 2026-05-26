package eventbus

import (
	"context"
	"time"
)

// Envelope representa el contrato de evento definido en la fase de diseño.
type Envelope struct {
	EventID        string                 `json:"event_id"`
	EventType      string                 `json:"event_type"`
	Version        string                 `json:"version"`
	Timestamp      time.Time              `json:"timestamp"`
	SourceService  string                 `json:"source_service"`
	IdempotencyKey string                 `json:"idempotency_key"`
	TraceID        string                 `json:"trace_id"`
	Payload        map[string]interface{} `json:"payload"`
	Metadata       map[string]string      `json:"metadata"`
}

// HandlerFunc es la firma que deben implementar los consumidores.
type HandlerFunc func(ctx context.Context, msg Envelope) error

// Publisher define la capacidad de emitir eventos.
type Publisher interface {
	Publish(ctx context.Context, topic string, msg Envelope) error
	Close() error
}

// Subscriber define la capacidad de consumir eventos.
type Subscriber interface {
	Subscribe(ctx context.Context, topic string, queueGroup string, handler HandlerFunc) error
	Close() error
}

// EventBus combina ambas capacidades.
type EventBus interface {
	Publisher
	Subscriber
}

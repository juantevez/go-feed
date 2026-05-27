package events

import (
	"time"

	"github.com/google/uuid"
)

// Envelope es el wrapper estándar para todo evento que circula por el bus.
// Implementa el contrato definido en el diseño funcional:
// event_id, event_type, version, timestamp, source_service,
// idempotency_key, trace_id, payload, metadata.
//
// Todos los servicios publican y consumen este tipo — nunca el payload crudo.
type Envelope struct {
	EventID        uuid.UUID `json:"event_id"`
	EventType      string    `json:"event_type"`      // e.g. "auth.user.registered.v1"
	Version        string    `json:"version"`         // e.g. "1.0.0"
	Timestamp      time.Time `json:"timestamp"`       // UTC, momento de generación
	SourceService  string    `json:"source_service"`  // e.g. "auth-service"
	IdempotencyKey string    `json:"idempotency_key"` // único por operación
	TraceID        string    `json:"trace_id"`        // propagado desde HTTP/worker
	Payload        any       `json:"payload"`
	Metadata       Metadata  `json:"metadata,omitempty"`
}

// Metadata es contexto operativo opcional: región, prioridad, client_type, etc.
// Nunca contiene datos de negocio — solo hints para enrutamiento y observabilidad.
type Metadata struct {
	Region     string `json:"region,omitempty"`
	ClientType string `json:"client_type,omitempty"` // "web", "mobile", "worker"
	Priority   string `json:"priority,omitempty"`    // "high", "normal", "low"
}

// Wrap envuelve un payload en un Envelope listo para publicar.
// El llamador debe proveer eventType y sourceService; el resto se genera.
func Wrap(eventType, sourceService, idempotencyKey, traceID string, payload any) Envelope {
	return Envelope{
		EventID:        uuid.New(),
		EventType:      eventType,
		Version:        "1.0.0",
		Timestamp:      time.Now().UTC(),
		SourceService:  sourceService,
		IdempotencyKey: idempotencyKey,
		TraceID:        traceID,
		Payload:        payload,
	}
}

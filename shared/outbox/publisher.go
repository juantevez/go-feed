package outbox

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/juantevez/my-ig/shared/events"
)

// Publisher escribe eventos en la tabla outbox. Implementa la misma firma que
// los EventPublisher de cada servicio, así los use cases no cambian de puerto.
//
// Debe llamarse dentro de TxManager.WithinTx: sin tx retorna ErrNoTx, porque
// insertar fuera de la tx de negocio rompería la atomicidad.
type Publisher struct {
	source string // source_service para payloads que no vienen envueltos
}

func NewPublisher(sourceService string) *Publisher {
	return &Publisher{source: sourceService}
}

func (p *Publisher) Publish(ctx context.Context, topic string, payload any) error {
	tx, ok := txFromContext(ctx)
	if !ok {
		return ErrNoTx
	}

	envelope, ok := payload.(events.Envelope)
	if !ok {
		envelope = events.Wrap(topic, p.source, "", "", payload)
	}

	data, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("outbox: marshal %s: %w", topic, err)
	}

	const q = `INSERT INTO outbox (id, topic, payload) VALUES ($1, $2, $3)`
	if _, err := tx.Exec(ctx, q, envelope.EventID, topic, data); err != nil {
		return fmt.Errorf("outbox: insert %s: %w", topic, err)
	}
	return nil
}

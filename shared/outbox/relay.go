package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/juantevez/my-ig/shared/telemetry"
)

var tracer = otel.Tracer("github.com/juantevez/my-ig/shared/outbox")

const (
	defaultPollInterval = 500 * time.Millisecond
	defaultBatchSize    = 100
	defaultRetention    = 7 * 24 * time.Hour
	cleanupInterval     = time.Hour
)

// Relay publica a JetStream los eventos pendientes de la tabla outbox.
//
// Garantías:
//   - at-least-once: un evento se marca publicado recién después del ack de JetStream.
//   - orden: se publica por created_at y el batch se corta en el primer error,
//     así un unfollow nunca sale antes que su follow.
//   - dedupe: cada mensaje lleva Nats-Msg-Id = event_id; si el relay publica y
//     cae antes de marcarlo, JetStream descarta el reenvío (ventana de duplicados
//     del stream, 2 min por defecto). Los consumers igual deben ser idempotentes.
//   - varias réplicas del servicio: FOR UPDATE SKIP LOCKED evita que dos relays
//     tomen las mismas filas.
type Relay struct {
	pool         *pgxpool.Pool
	js           nats.JetStreamContext
	pollInterval time.Duration
	batchSize    int
	retention    time.Duration
}

func NewRelay(pool *pgxpool.Pool, js nats.JetStreamContext) *Relay {
	return &Relay{
		pool:         pool,
		js:           js,
		pollInterval: defaultPollInterval,
		batchSize:    defaultBatchSize,
		retention:    defaultRetention,
	}
}

// Run procesa la outbox hasta que ctx se cancele.
func (r *Relay) Run(ctx context.Context) {
	slog.InfoContext(ctx, "outbox relay started", "poll_interval", r.pollInterval)

	poll := time.NewTicker(r.pollInterval)
	defer poll.Stop()
	cleanup := time.NewTicker(cleanupInterval)
	defer cleanup.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.InfoContext(ctx, "outbox relay stopped")
			return
		case <-poll.C:
			// Si el batch vino lleno puede haber más pendientes: seguimos sin esperar.
			for {
				n, err := r.processBatch(ctx)
				if err != nil {
					if ctx.Err() == nil {
						slog.ErrorContext(ctx, "outbox relay: batch failed", "err", err)
					}
					break
				}
				if n < r.batchSize {
					break
				}
			}
		case <-cleanup.C:
			r.cleanup(ctx)
		}
	}
}

type pending struct {
	id           uuid.UUID
	topic        string
	payload      []byte
	traceContext []byte // NULL en eventos de antes de la columna, o sin trace
}

// processBatch publica hasta batchSize eventos y retorna cuántos publicó.
func (r *Relay) processBatch(ctx context.Context) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	const selectQ = `
		SELECT id, topic, payload, trace_context
		FROM outbox
		WHERE published_at IS NULL
		ORDER BY created_at, id
		LIMIT $1
		FOR UPDATE SKIP LOCKED`

	rows, err := tx.Query(ctx, selectQ, r.batchSize)
	if err != nil {
		return 0, fmt.Errorf("select pending: %w", err)
	}
	var batch []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.topic, &p.payload, &p.traceContext); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan pending: %w", err)
		}
		batch = append(batch, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("read pending: %w", err)
	}
	if len(batch) == 0 {
		return 0, nil
	}

	published := make([]uuid.UUID, 0, len(batch))
	var publishErr error
	for _, p := range batch {
		err := r.publish(ctx, p)
		if err == nil {
			published = append(published, p.id)
			continue
		}

		publishErr = fmt.Errorf("publish %s (event_id=%s): %w", p.topic, p.id, err)
		const failQ = `UPDATE outbox SET attempts = attempts + 1, last_error = $2 WHERE id = $1`
		if _, dbErr := tx.Exec(ctx, failQ, p.id, err.Error()); dbErr != nil {
			return 0, fmt.Errorf("record failure: %w", dbErr)
		}
		break // cortar acá preserva el orden
	}

	if len(published) > 0 {
		const doneQ = `UPDATE outbox SET published_at = NOW() WHERE id = ANY($1)`
		if _, err := tx.Exec(ctx, doneQ, published); err != nil {
			return 0, fmt.Errorf("mark published: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}

	if publishErr != nil {
		return len(published), publishErr
	}
	return len(published), nil
}

// publish manda un evento a JetStream como hijo del trace que lo originó.
//
// El span "send" cuelga de la request que escribió el evento, aunque se
// publique después de que esa request terminó: en Tempo se ve el hueco entre
// el commit y la publicación, que es justamente el lag del outbox.
func (r *Relay) publish(ctx context.Context, p pending) error {
	ctx, span := tracer.Start(telemetry.UnmarshalTraceContext(ctx, p.traceContext), "send "+p.topic,
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "nats"),
			attribute.String("messaging.operation.type", "send"),
			attribute.String("messaging.destination.name", p.topic),
			attribute.String("messaging.message.id", p.id.String()),
		),
	)
	defer span.End()

	msg := nats.NewMsg(p.topic)
	msg.Data = p.payload
	telemetry.InjectNATS(ctx, msg.Header)

	if _, err := r.js.PublishMsg(msg, nats.MsgId(p.id.String()), nats.Context(ctx)); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// cleanup borra eventos ya publicados más viejos que retention.
func (r *Relay) cleanup(ctx context.Context) {
	const q = `DELETE FROM outbox WHERE published_at < NOW() - make_interval(secs => $1)`
	tag, err := r.pool.Exec(ctx, q, r.retention.Seconds())
	if err != nil {
		slog.ErrorContext(ctx, "outbox relay: cleanup failed", "err", err)
		return
	}
	if n := tag.RowsAffected(); n > 0 {
		slog.InfoContext(ctx, "outbox relay: cleanup", "deleted", n)
	}
}

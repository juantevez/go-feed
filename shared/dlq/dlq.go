// Package dlq implementa una dead letter queue sobre NATS JetStream.
//
// Política de reintentos (la maneja este paquete, no JetStream):
//   - el handler retorna nil → Ack.
//   - error transitorio → NakWithDelay con backoff, hasta MaxAttempts entregas.
//   - error permanente (Permanent) o agotó MaxAttempts → se publica en el
//     stream DLQ (subject "dlq.<subject original>") con el motivo en headers,
//     y recién con el ack de la DLQ se hace Term del original.
//
// El consumer JetStream se configura con MaxDeliver ilimitado: si la
// publicación a la DLQ falla, el mensaje se sigue reentregando en vez de
// descartarse en silencio.
package dlq

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	StreamName    = "DLQ"
	SubjectPrefix = "dlq."

	// MaxAttempts es la cantidad de entregas antes de mandar el mensaje a la DLQ.
	MaxAttempts = 3

	// Headers con el contexto de la falla.
	HeaderOriginalSubject = "Dlq-Original-Subject"
	HeaderConsumer        = "Dlq-Consumer"
	HeaderError           = "Dlq-Error"
	HeaderDeliveries      = "Dlq-Deliveries"
	HeaderStreamSeq       = "Dlq-Stream-Seq"
	HeaderFailedAt        = "Dlq-Failed-At"

	retention = 30 * 24 * time.Hour
)

// backoff entre reintentos, indexado por número de entrega (1-based).
var backoff = []time.Duration{time.Second, 5 * time.Second, 30 * time.Second}

// Handler procesa un mensaje. Retornar un error lo reintenta; envolverlo con
// Permanent lo manda directo a la DLQ (ej. payload que no se puede decodificar).
type Handler func(ctx context.Context, msg *nats.Msg) error

type permanentError struct{ err error }

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

// Permanent marca un error como no recuperable: reintentar no lo va a arreglar.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanentError{err: err}
}

func isPermanent(err error) bool {
	var p permanentError
	return errors.As(err, &p)
}

// EnsureStream crea el stream DLQ si no existe.
func EnsureStream(js nats.JetStreamContext) error {
	_, err := js.AddStream(&nats.StreamConfig{
		Name:     StreamName,
		Subjects: []string{SubjectPrefix + ">"},
		Storage:  nats.FileStorage,
		MaxAge:   retention,
	})
	if err != nil && !errors.Is(err, nats.ErrStreamNameAlreadyInUse) {
		return fmt.Errorf("dlq: ensure stream: %w", err)
	}
	return nil
}

// Subscribe crea (o reutiliza) un consumer durable sobre subject y despacha
// cada mensaje a h aplicando la política de reintentos + DLQ.
func Subscribe(js nats.JetStreamContext, subject, durable string, h Handler) error {
	if err := allowUnlimitedDeliveries(js, subject, durable); err != nil {
		return err
	}

	_, err := js.Subscribe(
		subject,
		wrap(js, subject, durable, h),
		nats.Durable(durable),
		nats.AckExplicit(),
		nats.MaxDeliver(-1), // los reintentos los cuenta wrap; ver doc del paquete
		nats.AckWait(30*time.Second),
		nats.DeliverNew(),
	)
	if err != nil {
		return fmt.Errorf("dlq: subscribe %s (%s): %w", subject, durable, err)
	}
	return nil
}

func wrap(js nats.JetStreamContext, subject, durable string, h Handler) nats.MsgHandler {
	return func(msg *nats.Msg) {
		err := h(context.Background(), msg)
		if err == nil {
			_ = msg.Ack()
			return
		}

		var deliveries uint64 = 1
		var streamSeq uint64
		if meta, mErr := msg.Metadata(); mErr == nil {
			deliveries = meta.NumDelivered
			streamSeq = meta.Sequence.Stream
		}

		if !isPermanent(err) && deliveries < MaxAttempts {
			delay := backoff[min(int(deliveries), len(backoff))-1]
			slog.Warn("consumer: handler failed, retrying",
				"subject", subject, "consumer", durable,
				"delivery", deliveries, "retry_in", delay, "err", err)
			_ = msg.NakWithDelay(delay)
			return
		}

		if pubErr := publish(js, msg, durable, err, deliveries, streamSeq); pubErr != nil {
			// No hacemos Term: sin DLQ el mensaje se perdería. Se reintenta.
			slog.Error("consumer: dead-letter publish failed, will retry",
				"subject", subject, "consumer", durable, "err", pubErr)
			_ = msg.NakWithDelay(backoff[len(backoff)-1])
			return
		}

		slog.Error("consumer: message dead-lettered",
			"subject", subject, "consumer", durable,
			"deliveries", deliveries, "permanent", isPermanent(err), "err", err)
		_ = msg.Term()
	}
}

func publish(js nats.JetStreamContext, msg *nats.Msg, durable string, cause error, deliveries, streamSeq uint64) error {
	dl := nats.NewMsg(SubjectPrefix + msg.Subject)
	dl.Data = msg.Data
	for k, v := range msg.Header {
		dl.Header[k] = v
	}
	dl.Header.Del(nats.MsgIdHdr) // el Nats-Msg-Id original haría que la DLQ lo dedupe con otro consumer
	dl.Header.Set(HeaderOriginalSubject, msg.Subject)
	dl.Header.Set(HeaderConsumer, durable)
	dl.Header.Set(HeaderError, cause.Error())
	dl.Header.Set(HeaderDeliveries, strconv.FormatUint(deliveries, 10))
	dl.Header.Set(HeaderStreamSeq, strconv.FormatUint(streamSeq, 10))
	dl.Header.Set(HeaderFailedAt, time.Now().UTC().Format(time.RFC3339))

	_, err := js.PublishMsg(dl)
	return err
}

// allowUnlimitedDeliveries migra consumers durables existentes creados con
// MaxDeliver finito: js.Subscribe falla si la config pedida no coincide.
func allowUnlimitedDeliveries(js nats.JetStreamContext, subject, durable string) error {
	stream, err := js.StreamNameBySubject(subject)
	if err != nil {
		return fmt.Errorf("dlq: stream for %s: %w", subject, err)
	}
	info, err := js.ConsumerInfo(stream, durable)
	if errors.Is(err, nats.ErrConsumerNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("dlq: consumer info %s: %w", durable, err)
	}
	if info.Config.MaxDeliver == -1 {
		return nil
	}
	cfg := info.Config
	cfg.MaxDeliver = -1
	if _, err := js.UpdateConsumer(stream, &cfg); err != nil {
		return fmt.Errorf("dlq: update consumer %s: %w", durable, err)
	}
	slog.Info("dlq: consumer migrated to unlimited deliveries", "consumer", durable)
	return nil
}

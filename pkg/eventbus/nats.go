package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

type NATSConfig struct {
	URL           string
	StreamName    string
	MaxReconnect  int
	ReconnectWait time.Duration
}

type NATSAdapter struct {
	conn *nats.Conn
	js   nats.JetStreamContext
	cfg  NATSConfig
	mu   sync.Mutex
	subs []*nats.Subscription
}

// NewNATSAdapter inicializa la conexión NATS + JetStream.
// Nota: nats.Context() como opción de Connect requiere v1.18+.
// Si tu versión es anterior, remueve esa línea y maneja el contexto en el publisher/consumer.
func NewNATSAdapter(ctx context.Context, cfg NATSConfig) (EventBus, error) {
	opts := []nats.Option{
		nats.Name("feed-eventbus"),
		nats.MaxReconnects(cfg.MaxReconnect),
		nats.ReconnectWait(cfg.ReconnectWait),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			log.Printf("[eventbus] disconnected: %v", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Printf("[eventbus] reconnected to %s", nc.ConnectedUrl())
		}),
	}

	nc, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("nats jetstream context: %w", err)
	}

	return &NATSAdapter{
		conn: nc,
		js:   js,
		cfg:  cfg,
	}, nil
}

// Publish envía un evento al tópico especificado.
// Usa PublishMsg con opciones de contexto para compatibilidad.
func (a *NATSAdapter) Publish(ctx context.Context, topic string, msg Envelope) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	// Crear mensaje NATS
	natsMsg := &nats.Msg{
		Subject: topic,
		Data:    data,
	}

	// Publicar con opciones de contexto (compatible con v1.31.0)
	// Si PublishMsgWithOptions da error, usar js.Publish(topic, data) sin contexto
	_, err = a.js.PublishMsg(natsMsg, nats.Context(ctx))
	if err != nil {
		return fmt.Errorf("publish to %s: %w", topic, err)
	}
	return nil
}

// Subscribe registra un consumidor en un tópico con queue group para load balancing.
func (a *NATSAdapter) Subscribe(ctx context.Context, topic, queueGroup string, handler HandlerFunc) error {
	// QueueSubscribe permite escalar consumidores horizontalmente
	sub, err := a.js.QueueSubscribe(topic, queueGroup, func(m *nats.Msg) {
		var env Envelope
		if err := json.Unmarshal(m.Data, &env); err != nil {
			log.Printf("[eventbus] decode error on %s: %v", topic, err)
			_ = m.Nak()
			return
		}

		// Ejecutar handler con contexto del subscriber
		if err := handler(ctx, env); err != nil {
			log.Printf("[eventbus] handler error for %s: %v", env.EventID, err)
			_ = m.Nak()
			return
		}

		// Ack exitoso
		_ = m.Ack()
	})
	if err != nil {
		return fmt.Errorf("subscribe to %s: %w", topic, err)
	}

	a.mu.Lock()
	a.subs = append(a.subs, sub)
	a.mu.Unlock()

	return nil
}

// Close realiza un drenado graceful de la conexión NATS.
func (a *NATSAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Unsubscribe de todos los subscribers
	for _, sub := range a.subs {
		_ = sub.Unsubscribe()
	}
	a.subs = nil

	// Drain cierra la conexión después de procesar mensajes en vuelo
	return a.conn.Drain()
}

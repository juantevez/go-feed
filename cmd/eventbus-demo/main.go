package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"feed-backend/pkg/eventbus"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 1. Inicializar bus
	bus, err := eventbus.NewNATSAdapter(ctx, eventbus.NATSConfig{
		URL:           os.Getenv("NATS_URL"),
		StreamName:    "FEED_DEMO_STREAM",
		MaxReconnect:  5,
		ReconnectWait: 2 * time.Second,
	})
	if err != nil {
		log.Fatalf("failed to init eventbus: %v", err)
	}

	// 2. Suscribirse a un tópico (queueGroup permite escalar consumidores horizontalmente)
	err = bus.Subscribe(ctx, "posts.created.v1", "feed-workers", func(ctx context.Context, msg eventbus.Envelope) error {
		log.Printf("✅ Consumed: %s | Type: %s | Payload: %v", msg.EventID, msg.EventType, msg.Payload)
		// Simular trabajo asíncrono
		select {
		case <-time.After(500 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	if err != nil {
		log.Fatalf("failed to subscribe: %v", err)
	}

	log.Println("🚀 Eventbus demo running. Publishing message in 2s...")
	time.Sleep(2 * time.Second)

	// 3. Publicar evento
	envelope := eventbus.Envelope{
		EventID:        uuid.New().String(),
		EventType:      "posts.created.v1",
		Version:        "1.0.0",
		Timestamp:      time.Now().UTC(),
		SourceService:  "content-service",
		IdempotencyKey: "req_demo_" + uuid.New().String(),
		TraceID:        uuid.New().String(),
		Payload: map[string]interface{}{
			"post_id":    uuid.New().String(),
			"author_id":  "user_123",
			"caption":    "Hello from NATS JetStream!",
			"created_at": time.Now().UTC().Format(time.RFC3339),
		},
		Metadata: map[string]string{
			"environment": "local",
		},
	}

	if err := bus.Publish(ctx, "posts.created.v1", envelope); err != nil {
		log.Printf("❌ Publish failed: %v", err)
	} else {
		log.Println("📤 Event published successfully")
	}

	// Esperar señal de cierre
	<-ctx.Done()
	log.Println("\n🔌 Shutting down gracefully...")
	if err := bus.Close(); err != nil {
		log.Printf("⚠️  Close error: %v", err)
	}
}

package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	// ✅ Alias para evitar colisión con net/http
	api "feed-backend/services/auth/internal/http"
	"feed-backend/services/auth/internal/repository"
	"feed-backend/services/auth/internal/usecase"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 1. Config
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://feed_dev:dev_password@localhost:5432/feed_db?sslmode=disable"
	}
	addr := os.Getenv("AUTH_PORT")
	if addr == "" {
		addr = ":8081"
	}

	// 2. Infraestructura
	repo, err := repository.NewPostgresRepo(ctx, dsn)
	if err != nil {
		log.Fatalf("failed to connect to DB: %v", err)
	}

	// 3. Use Cases
	uc := usecase.NewAuthUseCase(repo)

	// 4. HTTP (usando el alias 'api')
	handler := api.NewAuthHandler(uc)
	router := api.NewRouter(handler)

	srv := &http.Server{ // ← net/http estándar
		Addr:    addr,
		Handler: router,
	}

	// 5. Graceful shutdown
	go func() {
		log.Printf("🔐 Auth service starting on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("\n🔌 Shutting down auth service...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}
	log.Println("✅ Auth service stopped gracefully")
}

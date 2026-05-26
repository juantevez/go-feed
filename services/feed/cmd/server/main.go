package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	// Alias para evitar colisión con net/http
	"feed-backend/services/feed/internal/domain"
	api "feed-backend/services/feed/internal/http"
	"feed-backend/services/feed/internal/repository"
	"feed-backend/services/feed/internal/usecase"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Config
	redisAddr := os.Getenv("REDIS_URL")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	addr := os.Getenv("FEED_PORT")
	if addr == "" {
		addr = ":8082"
	}

	// 1. Redis Repo
	redisRepo, err := repository.NewRedisRepo(redisAddr, "", 0)
	if err != nil {
		log.Fatalf("redis init: %v", err)
	}

	// 2. Store compuesto (Redis + DB stub)
	store := &compositeStore{redis: redisRepo}

	// 3. UseCase & HTTP
	uc := usecase.NewFeedUseCase(store)
	handler := api.NewFeedHandler(uc)
	router := api.NewRouter(handler)

	// 4. Server
	srv := &http.Server{Addr: addr, Handler: router}
	go func() {
		log.Printf("📡 Feed service starting on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	log.Println("🟢 Feed service ready")
	<-ctx.Done()
	log.Println("\n🔌 Shutting down feed service...")

	// ✅ FIX 1: Shutdown solo recibe context.Context
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server forced to shutdown: %v", err)
	}
	log.Println("✅ Feed service stopped gracefully")
}

// compositeStore implementa domain.FeedStore
type compositeStore struct {
	redis *repository.RedisRepo
}

func (s *compositeStore) PushToFeed(ctx context.Context, u, p string, sc float64) error {
	return s.redis.PushToFeed(ctx, u, p, sc)
}
func (s *compositeStore) GetFeedPostIDs(ctx context.Context, u, c string, l int) ([]string, string, error) {
	return s.redis.GetFeedPostIDs(ctx, u, c, l)
}

// ✅ FIX 2: PostMeta está en domain, no en usecase
func (s *compositeStore) GetPostMeta(ctx context.Context, id string) (*domain.PostMeta, error) {
	// TODO: Reemplazar con query real a PostgreSQL
	return &domain.PostMeta{
		ID:        id,
		AuthorID:  "dev_user",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (s *compositeStore) GetUsername(ctx context.Context, id string) (string, error) {
	return "demo_user", nil
}

func (s *compositeStore) GetFollowers(ctx context.Context, authorID string) ([]string, error) {
	// TODO: Reemplazar con query real a tabla follows
	return []string{"follower_1", "follower_2"}, nil
}

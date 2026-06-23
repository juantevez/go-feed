package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

	"github.com/juantevez/my-ig/feed-service/configs"
	"github.com/juantevez/my-ig/feed-service/internal/application/usecase"
	"github.com/juantevez/my-ig/feed-service/internal/infrastructure/http/handler"
	"github.com/juantevez/my-ig/feed-service/internal/infrastructure/http/router"
	natsconsumer "github.com/juantevez/my-ig/feed-service/internal/infrastructure/messaging/natsconsumer"
	"github.com/juantevez/my-ig/feed-service/internal/infrastructure/persistence/postgres"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	// ── Config ───────────────────────────────────────────────────────────────
	cfg, err := configs.Load()
	if err != nil {
		slog.Error("config error", "err", err)
		os.Exit(1)
	}

	// ── Postgres ─────────────────────────────────────────────────────────────
	db, err := connectDB(cfg.DB)
	if err != nil {
		slog.Error("db connect error", "err", err)
		os.Exit(1)
	}
	defer db.Close()
	slog.Info("postgres connected")

	// ── NATS JetStream ───────────────────────────────────────────────────────
	js, nc, err := connectNATS(cfg.NATS)
	if err != nil {
		slog.Error("nats connect error", "err", err)
		os.Exit(1)
	}
	defer nc.Close()
	slog.Info("nats connected")

	// ── Adaptadores ──────────────────────────────────────────────────────────
	feedRepo := postgres.NewFeedRepository(db)
	followerRepo := postgres.NewFollowerRepository(db)

	// ── Use cases ────────────────────────────────────────────────────────────
	getFeedSvc := usecase.NewGetFeedService(feedRepo)
	fanOutSvc := usecase.NewFanOutService(feedRepo, followerRepo)

	// ── NATS streams ─────────────────────────────────────────────────────────
	if err := ensureStreams(js); err != nil {
		slog.Error("nats ensure streams error", "err", err)
		os.Exit(1)
	}

	// ── NATS consumer ────────────────────────────────────────────────────────
	consumer := natsconsumer.NewConsumer(js, fanOutSvc, feedRepo, followerRepo)
	if err := consumer.Subscribe(); err != nil {
		slog.Error("consumer subscribe error", "err", err)
		os.Exit(1)
	}
	slog.Info("nats consumers registered")

	// ── HTTP ─────────────────────────────────────────────────────────────────
	feedHandler := handler.NewFeedHandler(getFeedSvc)
	httpRouter := router.New(feedHandler)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.HTTP.Port),
		Handler:      httpRouter,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}

	// ── Graceful shutdown ────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("feed-service started", "port", cfg.HTTP.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-quit
	slog.Info("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("forced shutdown", "err", err)
	}
	slog.Info("server stopped")
}

func connectDB(cfg configs.DBConfig) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeout)
	defer cancel()

	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse db dsn: %w", err)
	}
	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return pool, nil
}

func ensureStreams(js nats.JetStreamContext) error {
	streams := []struct {
		name     string
		subjects []string
	}{
		{"POSTS", []string{"posts.>"}},
		{"AUTH", []string{"auth.>"}},
		{"USERS", []string{"users.>"}},
	}
	for _, s := range streams {
		_, err := js.AddStream(&nats.StreamConfig{
			Name:     s.name,
			Subjects: s.subjects,
			Storage:  nats.FileStorage,
			MaxAge:   7 * 24 * time.Hour,
		})
		if err != nil && !errors.Is(err, nats.ErrStreamNameAlreadyInUse) {
			return fmt.Errorf("ensure stream %s: %w", s.name, err)
		}
	}
	return nil
}

func connectNATS(cfg configs.NATSConfig) (nats.JetStreamContext, *nats.Conn, error) {
	nc, err := nats.Connect(cfg.URL,
		nats.Timeout(cfg.ConnectTimeout),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(5),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("nats connect: %w", err)
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("nats jetstream: %w", err)
	}
	return js, nc, nil
}

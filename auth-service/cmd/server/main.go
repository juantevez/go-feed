package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

	"github.com/juantevez/my-ig/auth-service/configs"
	"github.com/juantevez/my-ig/auth-service/internal/application/usecase"
	"github.com/juantevez/my-ig/auth-service/internal/infrastructure/http/handler"
	"github.com/juantevez/my-ig/auth-service/internal/infrastructure/http/router"
	jwtadapter "github.com/juantevez/my-ig/auth-service/internal/infrastructure/jwt"
	natsadapter "github.com/juantevez/my-ig/auth-service/internal/infrastructure/messaging/nats"
	"github.com/juantevez/my-ig/auth-service/internal/infrastructure/persistence/postgres"
)

func main() {
	// ── Logger ───────────────────────────────────────────────────────────────
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

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

	// ── Adaptadores (driven ports) ───────────────────────────────────────────
	userRepo := postgres.NewUserRepository(db)
	refreshTokenRepo := postgres.NewRefreshTokenRepository(db)
	publisher := natsadapter.NewPublisher(js)
	tokenSvc := jwtadapter.New(
		cfg.JWT.Secret,
		cfg.JWT.AccessTokenTTL,
		cfg.JWT.RefreshTokenTTL,
		refreshTokenRepo,
	)

	// ── Caso de uso ──────────────────────────────────────────────────────────
	authSvc := usecase.NewAuthService(userRepo, tokenSvc, publisher)

	// ── HTTP ─────────────────────────────────────────────────────────────────
	authHandler := handler.NewAuthHandler(authSvc)
	httpRouter := router.New(authHandler, tokenSvc)

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
		slog.Info("auth-service started", "port", cfg.HTTP.Port)
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

// ── helpers de conexión ───────────────────────────────────────────────────────

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

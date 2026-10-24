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

	"github.com/juantevez/my-ig/notification-service/configs"
	"github.com/juantevez/my-ig/notification-service/internal/application/usecase"
	httphandler "github.com/juantevez/my-ig/notification-service/internal/infrastructure/http/handler"
	"github.com/juantevez/my-ig/notification-service/internal/infrastructure/http/router"
	natsconsumer "github.com/juantevez/my-ig/notification-service/internal/infrastructure/messaging/nats"
	"github.com/juantevez/my-ig/notification-service/internal/infrastructure/persistence/postgres"
	"github.com/juantevez/my-ig/notification-service/internal/infrastructure/ws"
	"github.com/juantevez/my-ig/shared/outbox"
	"github.com/juantevez/my-ig/shared/revocation"
	"github.com/juantevez/my-ig/shared/telemetry"
)

func main() {
	slog.SetDefault(slog.New(telemetry.LogHandler(slog.NewJSONHandler(os.Stdout, nil))))

	// ── Telemetría ───────────────────────────────────────────────────────────
	// Antes del pool y del router: los instrumentadores toman los providers globales.
	shutdownTelemetry, err := telemetry.Init(context.Background(), "notification-service")
	if err != nil {
		slog.Error("telemetry init error", "err", err)
		os.Exit(1)
	}

	cfg, err := configs.Load()
	if err != nil {
		slog.Error("config error", "err", err)
		os.Exit(1)
	}

	// ── Postgres ──────────────────────────────────────────────────────────────
	db, err := connectDB(cfg.DB)
	if err != nil {
		slog.Error("db connect error", "err", err)
		os.Exit(1)
	}
	defer db.Close()
	slog.Info("postgres connected")

	// ── NATS ──────────────────────────────────────────────────────────────────
	js, nc, err := connectNATS(cfg.NATS)
	if err != nil {
		slog.Error("nats connect error", "err", err)
		os.Exit(1)
	}
	defer nc.Close()
	slog.Info("nats connected")

	// ── Adaptadores ───────────────────────────────────────────────────────────
	notifRepo := postgres.NewNotificationRepository(db)
	followStates := postgres.NewFollowStateRepository(db)
	hub := ws.NewHub()

	// ── Use case ──────────────────────────────────────────────────────────────
	notifSvc := usecase.NewNotificationService(outbox.NewTxManager(db), notifRepo, followStates, hub)

	// ── NATS consumer ─────────────────────────────────────────────────────────
	consumer := natsconsumer.NewConsumer(js, notifSvc)
	if err := consumer.Subscribe(); err != nil {
		slog.Error("consumer subscribe error", "err", err)
		os.Exit(1)
	}
	slog.Info("nats consumers registered")

	// ── HTTP + WebSocket ───────────────────────────────────────────────────────
	notifHandler := httphandler.NewNotificationHandler(notifSvc, hub)
	// ── Revocación de tokens (logout) ─────────────────────────────────────────
	denylist, err := revocation.New(js)
	if err != nil {
		slog.Error("revocation denylist error", "err", err)
		os.Exit(1)
	}
	defer denylist.Stop()
	// Un logout cierra en el momento los WebSockets abiertos con ese token.
	denylist.OnRevoke(hub.RevokeToken)
	httpRouter := router.New(notifHandler, cfg.JWT.Secret, denylist)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.HTTP.Port),
		Handler:      telemetry.HTTPHandler(httpRouter),
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("notification-service started", "port", cfg.HTTP.Port)
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
	if err := shutdownTelemetry(ctx); err != nil {
		slog.Error("telemetry shutdown error", "err", err)
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
	poolCfg.ConnConfig.Tracer = telemetry.PgxTracer()
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
		nats.MaxReconnects(-1), // reconectar siempre: con 5 intentos, un corte corto de NATS dejaba el servicio desconectado para siempre
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

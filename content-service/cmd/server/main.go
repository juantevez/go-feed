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

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

	"github.com/juantevez/my-ig/content-service/configs"
	"github.com/juantevez/my-ig/content-service/internal/application/usecase"
	"github.com/juantevez/my-ig/content-service/internal/infrastructure/http/handler"
	"github.com/juantevez/my-ig/content-service/internal/infrastructure/http/router"
	natspublisher "github.com/juantevez/my-ig/content-service/internal/infrastructure/messaging/nats"
	"github.com/juantevez/my-ig/content-service/internal/infrastructure/persistence/postgres"
	s3store "github.com/juantevez/my-ig/content-service/internal/infrastructure/storage/s3"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

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

	// ── NATS ──────────────────────────────────────────────────────────────────
	js, nc, err := connectNATS(cfg.NATS)
	if err != nil {
		slog.Error("nats connect error", "err", err)
		os.Exit(1)
	}
	defer nc.Close()

	// ── S3 / MinIO ────────────────────────────────────────────────────────────
	s3Client, err := connectS3(cfg.S3)
	if err != nil {
		slog.Error("s3 connect error", "err", err)
		os.Exit(1)
	}

	// ── Adaptadores ───────────────────────────────────────────────────────────
	postRepo := postgres.NewPostRepository(db)
	mediaStore := s3store.New(s3Client, cfg.S3.Bucket, cfg.S3.BaseURL)
	publisher := natspublisher.New(js)

	// ── Use case ──────────────────────────────────────────────────────────────
	postSvc := usecase.NewPostService(postRepo, mediaStore, publisher)

	// ── HTTP ──────────────────────────────────────────────────────────────────
	postHandler := handler.NewPostHandler(postSvc)
	httpRouter := router.New(postHandler, cfg.JWT.Secret)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.HTTP.Port),
		Handler:      httpRouter,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("content-service started", "port", cfg.HTTP.Port)
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

func connectS3(cfg configs.S3Config) (*awss3.Client, error) {
	optFns := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		),
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), optFns...)
	if err != nil {
		return nil, fmt.Errorf("aws config: %w", err)
	}

	clientOpts := []func(*awss3.Options){}
	if cfg.Endpoint != "" {
		// MinIO o cualquier S3-compatible local
		clientOpts = append(clientOpts, func(o *awss3.Options) {
			o.BaseEndpoint = &cfg.Endpoint
			o.UsePathStyle = true // MinIO requiere path-style
		})
	}

	return awss3.NewFromConfig(awsCfg, clientOpts...), nil
}

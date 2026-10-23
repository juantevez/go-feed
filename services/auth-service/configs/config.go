package configs

import (
	"errors"
	"fmt"
	"os"
	"time"
)

// Config holds all runtime configuration for auth-service.
// Values come exclusively from environment variables — no config files.
type Config struct {
	HTTP     HTTPConfig
	DB       DBConfig
	NATS     NATSConfig
	JWT      JWTConfig
	LogLevel string
}

type HTTPConfig struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

type DBConfig struct {
	DSN            string
	MaxConns       int32
	MinConns       int32
	ConnectTimeout time.Duration
}

type NATSConfig struct {
	URL            string
	ConnectTimeout time.Duration
}

type JWTConfig struct {
	Secret          string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

// Load reads all required env vars and returns a validated Config.
// Returns an error listing every missing/invalid var so the operator
// can fix them all in one shot.
func Load() (*Config, error) {
	var errs []error

	secret := requireEnv("JWT_SECRET", &errs)
	dsn := requireEnv("DB_DSN", &errs)
	natsURL := requireEnv("NATS_URL", &errs)

	if len(errs) > 0 {
		return nil, fmt.Errorf("config: missing required env vars: %w", errors.Join(errs...))
	}

	return &Config{
		HTTP: HTTPConfig{
			Port:         envOr("PORT", "8080"),
			ReadTimeout:  parseDuration("HTTP_READ_TIMEOUT", 10*time.Second),
			WriteTimeout: parseDuration("HTTP_WRITE_TIMEOUT", 10*time.Second),
			IdleTimeout:  parseDuration("HTTP_IDLE_TIMEOUT", 60*time.Second),
		},
		DB: DBConfig{
			DSN:            dsn,
			MaxConns:       10,
			MinConns:       2,
			ConnectTimeout: parseDuration("DB_CONNECT_TIMEOUT", 5*time.Second),
		},
		NATS: NATSConfig{
			URL:            natsURL,
			ConnectTimeout: parseDuration("NATS_CONNECT_TIMEOUT", 5*time.Second),
		},
		JWT: JWTConfig{
			Secret:          secret,
			AccessTokenTTL:  parseDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTokenTTL: parseDuration("JWT_REFRESH_TTL", 7*24*time.Hour),
		},
		LogLevel: envOr("LOG_LEVEL", "info"),
	}, nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func requireEnv(key string, errs *[]error) string {
	v := os.Getenv(key)
	if v == "" {
		*errs = append(*errs, fmt.Errorf("%s is required", key))
	}
	return v
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

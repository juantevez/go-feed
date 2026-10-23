package configs

import (
	"errors"
	"fmt"
	"os"
	"time"
)

type Config struct {
	HTTP HTTPConfig
	DB   DBConfig
	NATS NATSConfig
	JWT  JWTConfig
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
	Secret string
}

func Load() (*Config, error) {
	var errs []error
	dsn := requireEnv("DB_DSN", &errs)
	natsURL := requireEnv("NATS_URL", &errs)
	secret := requireEnv("JWT_SECRET", &errs)

	if len(errs) > 0 {
		return nil, fmt.Errorf("config: %w", errors.Join(errs...))
	}

	return &Config{
		HTTP: HTTPConfig{
			Port:         envOr("PORT", "8084"),
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
		JWT: JWTConfig{Secret: secret},
	}, nil
}

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
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

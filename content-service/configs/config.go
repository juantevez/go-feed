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
	S3   S3Config
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

type S3Config struct {
	Bucket   string
	Region   string
	Endpoint string // para MinIO local; vacío en AWS real
	BaseURL  string // URL pública del bucket
}

type JWTConfig struct {
	Secret string // debe ser el mismo que usa auth-service
}

func Load() (*Config, error) {
	var errs []error

	dsn := requireEnv("DB_DSN", &errs)
	natsURL := requireEnv("NATS_URL", &errs)
	bucket := requireEnv("S3_BUCKET", &errs)
	region := requireEnv("S3_REGION", &errs)
	baseURL := requireEnv("S3_BASE_URL", &errs)
	jwtSecret := requireEnv("JWT_SECRET", &errs)

	if len(errs) > 0 {
		return nil, fmt.Errorf("config: %w", errors.Join(errs...))
	}

	return &Config{
		HTTP: HTTPConfig{
			Port:         envOr("PORT", "8082"),
			ReadTimeout:  parseDuration("HTTP_READ_TIMEOUT", 30*time.Second), // más largo por uploads
			WriteTimeout: parseDuration("HTTP_WRITE_TIMEOUT", 30*time.Second),
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
		S3: S3Config{
			Bucket:   bucket,
			Region:   region,
			Endpoint: os.Getenv("S3_ENDPOINT"), // vacío en AWS, seteado en local con MinIO
			BaseURL:  baseURL,
		},
		JWT: JWTConfig{Secret: jwtSecret},
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

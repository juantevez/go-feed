package idempotency

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisStore struct {
	client *redis.Client
	cfg    Config
}

// NewRedisStore crea una nueva instancia del store Redis.
func NewRedisStore(cfg Config) (Store, error) {
	if cfg.KeyTTL == 0 {
		cfg.KeyTTL = 24 * time.Hour
	}
	if cfg.PendingTTL == 0 {
		cfg.PendingTTL = 30 * time.Second
	}
	if cfg.Prefix == "" {
		cfg.Prefix = KeyPrefix
	}

	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// Verificar conexión
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	return &RedisStore{client: client, cfg: cfg}, nil
}

func (s *RedisStore) key(name string) string {
	return s.cfg.Prefix + name
}

// Get recupera un record por clave.
func (s *RedisStore) Get(ctx context.Context, key string) (*Record, error) {
	data, err := s.client.Get(ctx, s.key(key)).Bytes()
	if err == redis.Nil {
		return nil, nil // No encontrado
	}
	if err != nil {
		return nil, fmt.Errorf("redis get: %w", err)
	}

	var rec Record
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("unmarshal record: %w", err)
	}

	// Verificar expiración
	if time.Now().After(rec.ExpiresAt) {
		_ = s.Delete(ctx, key) // Limpieza lazy
		return nil, ErrKeyExpired
	}

	return &rec, nil
}

// SetPending intenta adquirir un lock para la clave.
// Usa SET NX para atomicidad.
func (s *RedisStore) SetPending(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	rec := Record{
		Key:         key,
		Status:      StatusPending,
		ExpiresAt:   time.Now().Add(ttl),
		LastUpdated: time.Now(),
	}

	data, err := json.Marshal(rec)
	if err != nil {
		return false, fmt.Errorf("marshal record: %w", err)
	}

	// SET key value NX EX ttl → solo si no existe
	ok, err := s.client.SetNX(ctx, s.key(key), data, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("redis setnx: %w", err)
	}
	if !ok {
		return false, ErrKeyAlreadyExists
	}
	return true, nil
}

// SetCompleted cachea la respuesta exitosa.
func (s *RedisStore) SetCompleted(ctx context.Context, key string, resp Response, ttl time.Duration) error {
	rec := Record{
		Key:         key,
		Status:      StatusCompleted,
		Response:    &resp,
		ExpiresAt:   time.Now().Add(ttl),
		LastUpdated: time.Now(),
	}

	return s.setRecord(ctx, rec, ttl)
}

// SetFailed registra el error y permite reintentos.
func (s *RedisStore) SetFailed(ctx context.Context, key string, err error, ttl time.Duration) error {
	rec := Record{
		Key:         key,
		Status:      StatusFailed,
		Error:       err.Error(),
		ExpiresAt:   time.Now().Add(ttl),
		LastUpdated: time.Now(),
	}

	return s.setRecord(ctx, rec, ttl)
}

// setRecord es un helper para persistir un record.
func (s *RedisStore) setRecord(ctx context.Context, rec Record, ttl time.Duration) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal record: %w", err)
	}

	if err := s.client.Set(ctx, s.key(rec.Key), data, ttl).Err(); err != nil {
		return fmt.Errorf("redis set: %w", err)
	}
	return nil
}

// Delete elimina una clave manualmente.
func (s *RedisStore) Delete(ctx context.Context, key string) error {
	return s.client.Del(ctx, s.key(key)).Err()
}

// Close libera la conexión Redis.
func (s *RedisStore) Close() error {
	return s.client.Close()
}

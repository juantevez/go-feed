package idempotency

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// TestRedisStore_Integration prueba el store con Redis real (requiere docker).
func TestRedisStore_Integration(t *testing.T) {
	// Skip si no hay Redis disponible
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Skip("Redis not available: skipping integration test")
	}
	defer client.Close()

	cfg := Config{
		Addr:       "localhost:6379",
		KeyTTL:     1 * time.Hour,
		PendingTTL: 10 * time.Second,
		Prefix:     "test:idemp:",
	}

	store, err := NewRedisStore(cfg)
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	key := "test_key_123"

	// 1. SetPending debe funcionar
	acquired, err := store.SetPending(ctx, key, cfg.PendingTTL)
	if err != nil || !acquired {
		t.Fatalf("SetPending: acquired=%v, err=%v", acquired, err)
	}

	// 2. Segundo SetPending debe fallar con ErrKeyAlreadyExists
	acquired, err = store.SetPending(ctx, key, cfg.PendingTTL)
	if err != ErrKeyAlreadyExists || acquired {
		t.Fatalf("SetPending duplicate: expected ErrKeyAlreadyExists, got %v", err)
	}

	// 3. SetCompleted debe actualizar el estado
	resp := Response{
		StatusCode: 201,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       []byte(`{"id":"abc123"}`),
		CreatedAt:  time.Now(),
	}
	if err := store.SetCompleted(ctx, key, resp, cfg.KeyTTL); err != nil {
		t.Fatalf("SetCompleted: %v", err)
	}

	// 4. Get debe retornar el record con respuesta cacheada
	record, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if record.Status != StatusCompleted {
		t.Errorf("Status = %v, want %v", record.Status, StatusCompleted)
	}
	if record.Response.StatusCode != 201 {
		t.Errorf("Response.StatusCode = %v, want 201", record.Response.StatusCode)
	}

	// 5. Delete debe remover la clave
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	record, err = store.Get(ctx, key)
	if err != nil || record != nil {
		t.Errorf("After Delete: record = %v, want nil", record)
	}
}

// TestMiddleware_Handler prueba el middleware con requests simulados.
func TestMiddleware_Handler(t *testing.T) {
	// Usar store en memoria para tests (implementación simplificada)
	store := &memoryStore{data: make(map[string]*Record)}

	mw := NewMiddleware(MiddlewareConfig{
		Store: store,
	})

	// Handler de prueba que siempre retorna 201
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"created":true}`))
	})

	handler := mw.Handler(next)

	// Primer request: debe procesarse normalmente
	req1 := httptest.NewRequest("POST", "/posts", nil)
	req1.Header.Set(HeaderIdempotencyKey, "key_abc_123")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusCreated {
		t.Errorf("First request: status = %d, want %d", rec1.Code, http.StatusCreated)
	}
	if rec1.Header().Get(HeaderIdempotent) != "" {
		t.Errorf("First request: X-Idempotent should be empty, got %v", rec1.Header().Get(HeaderIdempotent))
	}

	// Segundo request con misma clave: debe retornar respuesta cacheada
	req2 := httptest.NewRequest("POST", "/posts", nil)
	req2.Header.Set(HeaderIdempotencyKey, "key_abc_123")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusCreated {
		t.Errorf("Second request: status = %d, want %d", rec2.Code, http.StatusCreated)
	}
	if rec2.Header().Get(HeaderIdempotent) != "true" {
		t.Errorf("Second request: X-Idempotent = %v, want 'true'", rec2.Header().Get(HeaderIdempotent))
	}
}

// memoryStore es una implementación en memoria para tests (no thread-safe).
type memoryStore struct {
	data map[string]*Record
}

func (s *memoryStore) Get(ctx context.Context, key string) (*Record, error) {
	rec := s.data[key]
	if rec == nil {
		return nil, nil
	}
	if time.Now().After(rec.ExpiresAt) {
		delete(s.data, key)
		return nil, ErrKeyExpired
	}
	return rec, nil
}

func (s *memoryStore) SetPending(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if s.data[key] != nil {
		return false, ErrKeyAlreadyExists
	}
	s.data[key] = &Record{
		Key: key, Status: StatusPending,
		ExpiresAt: time.Now().Add(ttl), LastUpdated: time.Now(),
	}
	return true, nil
}

func (s *memoryStore) SetCompleted(ctx context.Context, key string, resp Response, ttl time.Duration) error {
	s.data[key] = &Record{
		Key: key, Status: StatusCompleted, Response: &resp,
		ExpiresAt: time.Now().Add(ttl), LastUpdated: time.Now(),
	}
	return nil
}

func (s *memoryStore) SetFailed(ctx context.Context, key string, err error, ttl time.Duration) error {
	s.data[key] = &Record{
		Key: key, Status: StatusFailed, Error: err.Error(),
		ExpiresAt: time.Now().Add(ttl), LastUpdated: time.Now(),
	}
	return nil
}

func (s *memoryStore) Delete(ctx context.Context, key string) error {
	delete(s.data, key)
	return nil
}

func (s *memoryStore) Close() error { return nil }

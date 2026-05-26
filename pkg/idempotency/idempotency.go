package idempotency

import (
	"context"
	"errors"
	"time"
)

// Status representa el estado de una clave de idempotencia.
type Status string

const (
	StatusPending   Status = "pending"   // Procesamiento en curso
	StatusCompleted Status = "completed" // Éxito, respuesta cacheada
	StatusFailed    Status = "failed"    // Error, permite reintento
)

// KeyPrefix es el prefijo usado en Redis para todas las claves.
const KeyPrefix = "idemp:"

// Response representa la respuesta cacheada de una operación idempotente.
type Response struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
	CreatedAt  time.Time
}

// Record almacena el estado completo de una clave de idempotencia.
type Record struct {
	Key         string
	Status      Status
	Response    *Response
	Error       string
	ExpiresAt   time.Time
	LastUpdated time.Time
}

// Store define la interfaz para persistir claves de idempotencia.
type Store interface {
	// Get recupera un record por clave. Retorna nil si no existe.
	Get(ctx context.Context, key string) (*Record, error)

	// SetPending marca una clave como en procesamiento.
	// Retorna true si se logró adquirir el lock, false si ya existe.
	SetPending(ctx context.Context, key string, ttl time.Duration) (bool, error)

	// SetCompleted marca una clave como exitosa y cachea la respuesta.
	SetCompleted(ctx context.Context, key string, resp Response, ttl time.Duration) error

	// SetFailed marca una clave como fallida, permitiendo reintentos.
	SetFailed(ctx context.Context, key string, err error, ttl time.Duration) error

	// Delete elimina una clave manualmente (para cleanup o testing).
	Delete(ctx context.Context, key string) error

	// Close libera recursos de conexión.
	Close() error
}

// Config contiene la configuración para el store Redis.
type Config struct {
	Addr       string        // "localhost:6379"
	Password   string        // opcional
	DB         int           // 0-15, default 0
	KeyTTL     time.Duration // TTL para claves COMPLETED/FAILED (default: 24h)
	PendingTTL time.Duration // TTL para claves PENDING (default: 30s)
	Prefix     string        // Prefijo personalizado (default: "idemp:")
}

// ErrKeyAlreadyExists se retorna cuando SetPending falla porque la clave ya está en uso.
var ErrKeyAlreadyExists = errors.New("idempotency: key already processing")

// ErrKeyExpired se retorna cuando se intenta usar una clave que expiró.
var ErrKeyExpired = errors.New("idempotency: key expired")

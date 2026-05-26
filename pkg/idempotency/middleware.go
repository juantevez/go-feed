package idempotency

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	// HeaderIdempotencyKey es el header que los clientes deben enviar.
	HeaderIdempotencyKey = "Idempotency-Key"

	// HeaderIdempotent indica si la respuesta fue servida desde caché.
	HeaderIdempotent = "X-Idempotent"
)

// Middleware configura el middleware de idempotencia.
type Middleware struct {
	store        Store
	group        *singleflight.Group
	skipPrefixes []string // Paths a excluir (ej. "/health", "/metrics")
}

// MiddlewareConfig contiene la configuración del middleware.
type MiddlewareConfig struct {
	Store        Store
	SkipPrefixes []string // opcional: paths que no requieren idempotencia
}

// NewMiddleware crea una nueva instancia del middleware.
func NewMiddleware(cfg MiddlewareConfig) *Middleware {
	return &Middleware{
		store:        cfg.Store,
		group:        &singleflight.Group{},
		skipPrefixes: cfg.SkipPrefixes,
	}
}

// ShouldSkip verifica si el path debe excluirse del middleware.
func (m *Middleware) ShouldSkip(path string) bool {
	for _, prefix := range m.skipPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// Handler es el middleware HTTP compatible con net/http.
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Solo aplicar a métodos con efectos de escritura
		if !isWriteMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}

		// Skip paths configurados
		if m.ShouldSkip(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// Extraer clave de idempotencia
		key := r.Header.Get(HeaderIdempotencyKey)
		if key == "" {
			// Sin clave: proceder normal (o retornar 400 si es requerida)
			next.ServeHTTP(w, r)
			return
		}

		// Validar formato de clave (opcional pero recomendado)
		if !isValidKey(key) {
			http.Error(w, `{"error":"invalid idempotency key format"}`, http.StatusBadRequest)
			return
		}

		ctx := r.Context()

		// 1. Intentar recuperar respuesta cacheada
		record, err := m.store.Get(ctx, key)
		if err != nil {
			http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			return
		}

		// 2. Si está COMPLETED, retornar respuesta cacheada
		if record != nil && record.Status == StatusCompleted && record.Response != nil {
			for k, v := range record.Response.Headers {
				w.Header().Set(k, v)
			}
			w.Header().Set(HeaderIdempotent, "true")
			w.WriteHeader(record.Response.StatusCode)
			_, _ = w.Write(record.Response.Body)
			return
		}

		// 3. Si está PENDING, usar singleflight para evitar procesamiento duplicado
		if record != nil && record.Status == StatusPending {
			// Esperar a que otro request complete (con timeout)
			waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			_, err, _ := m.group.Do(key, func() (interface{}, error) {
				// Este bloque se ejecuta solo si somos el "líder"
				// Si ya hay un líder, esperamos y luego re-chequeamos
				time.Sleep(100 * time.Millisecond)
				return nil, nil
			})
			fmt.Print(err)

			// Re-chequear después de esperar
			record, _ = m.store.Get(waitCtx, key)
			if record != nil && record.Status == StatusCompleted && record.Response != nil {
				for k, v := range record.Response.Headers {
					w.Header().Set(k, v)
				}
				w.Header().Set(HeaderIdempotent, "true")
				w.WriteHeader(record.Response.StatusCode)
				_, _ = w.Write(record.Response.Body)
				return
			}

			// Si aún está pending o falló, retornar 409 Conflict
			http.Error(w, `{"error":"request already processing","retry_after":5}`, http.StatusConflict)
			return
		}

		// 4. Intentar adquirir lock PENDING
		acquired, err := m.store.SetPending(ctx, key, m.store.(*RedisStore).cfg.PendingTTL)
		if err == ErrKeyAlreadyExists {
			// Otro request ganó la carrera: retornar 409
			http.Error(w, `{"error":"request already processing","retry_after":5}`, http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			return
		}
		if !acquired {
			// Safety fallback
			http.Error(w, `{"error":"could not acquire idempotency lock"}`, http.StatusServiceUnavailable)
			return
		}

		// 5. Capturar respuesta del handler siguiente
		cw := &captureWriter{ResponseWriter: w, body: &bytes.Buffer{}}
		next.ServeHTTP(cw, r)

		// 6. Cachea respuesta si fue exitosa (2xx)
		if cw.statusCode >= 200 && cw.statusCode < 300 {
			resp := Response{
				StatusCode: cw.statusCode,
				Headers:    make(map[string]string),
				Body:       cw.body.Bytes(),
				CreatedAt:  time.Now(),
			}
			// Copiar headers relevantes (excluir algunos dinámicos)
			for k, v := range cw.Header() {
				if len(v) > 0 && !isDynamicHeader(k) {
					resp.Headers[k] = v[0]
				}
			}
			_ = m.store.SetCompleted(ctx, key, resp, m.store.(*RedisStore).cfg.KeyTTL)
		} else if cw.statusCode >= 400 {
			// Marcar como failed para permitir reintentos
			_ = m.store.SetFailed(ctx, key, fmt.Errorf("http %d", cw.statusCode), m.store.(*RedisStore).cfg.KeyTTL)
		}
	})
}

// isWriteMethod verifica si el método HTTP tiene efectos de escritura.
func isWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// isValidKey valida el formato de la clave de idempotencia.
func isValidKey(key string) bool {
	// Ejemplo: permitir alfanumérico, guiones, underscores, máximo 128 chars
	if len(key) == 0 || len(key) > 128 {
		return false
	}
	for _, r := range key {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

// isDynamicHeader indica headers que no deben cachease (varían por request).
func isDynamicHeader(name string) bool {
	lower := strings.ToLower(name)
	switch lower {
	case "date", "server", "x-request-id", "x-trace-id", "content-length":
		return true
	default:
		return false
	}
}

// captureWriter envuelve http.ResponseWriter para capturar statusCode y body.
type captureWriter struct {
	http.ResponseWriter
	body       *bytes.Buffer
	statusCode int
	written    bool
}

func (cw *captureWriter) WriteHeader(code int) {
	if cw.written {
		return
	}
	cw.statusCode = code
	cw.written = true
	cw.ResponseWriter.WriteHeader(code)
}

func (cw *captureWriter) Write(b []byte) (int, error) {
	if !cw.written {
		cw.WriteHeader(http.StatusOK)
	}
	cw.body.Write(b)
	return cw.ResponseWriter.Write(b)
}

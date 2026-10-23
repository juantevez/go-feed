// Package revocation mantiene la lista de access tokens revocados (logout).
//
// Los access tokens son stateless y los valida cada servicio por su cuenta,
// así que la revocación tiene que llegar a todos. La lista vive en un bucket
// KV de NATS JetStream: auth-service escribe el jti al hacer logout y cada
// servicio mantiene una copia en memoria vía Watch, sin roundtrip por request.
package revocation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	Bucket = "REVOKED_TOKENS"

	// bucketTTL descarta entradas viejas del bucket. Tiene que ser mayor que el
	// TTL del access token: pasado su exp, el token ya es inválido por sí solo.
	bucketTTL = 24 * time.Hour

	pruneInterval = time.Minute
)

// Denylist es la vista local de los tokens revocados. Segura para uso concurrente.
type Denylist struct {
	kv      nats.KeyValue
	watcher nats.KeyWatcher

	mu        sync.RWMutex
	revoked   map[string]time.Time // jti → exp del token
	listeners []func(jti string)
	stop      chan struct{}
}

// New se conecta (o crea) el bucket y carga todas las revocaciones vigentes
// antes de retornar, así un servicio recién arrancado no acepta tokens ya revocados.
func New(js nats.JetStreamContext) (*Denylist, error) {
	kv, err := js.KeyValue(Bucket)
	if errors.Is(err, nats.ErrBucketNotFound) {
		kv, err = js.CreateKeyValue(&nats.KeyValueConfig{
			Bucket:  Bucket,
			TTL:     bucketTTL,
			Storage: nats.FileStorage,
		})
	}
	if err != nil {
		return nil, fmt.Errorf("revocation: bucket %s: %w", Bucket, err)
	}

	w, err := kv.WatchAll(nats.IgnoreDeletes())
	if err != nil {
		return nil, fmt.Errorf("revocation: watch: %w", err)
	}

	d := &Denylist{kv: kv, watcher: w, revoked: make(map[string]time.Time), stop: make(chan struct{})}

	// El watcher entrega primero los valores existentes y después un nil.
	for entry := range w.Updates() {
		if entry == nil {
			break
		}
		d.apply(entry)
	}

	go d.run()
	return d, nil
}

// Revoke marca el jti como revocado hasta exp (el vencimiento del token).
func (d *Denylist) Revoke(_ context.Context, jti string, exp time.Time) error {
	if _, err := d.kv.Put(jti, []byte(exp.UTC().Format(time.RFC3339))); err != nil {
		return fmt.Errorf("revocation: put %s: %w", jti, err)
	}
	d.set(jti, exp) // sin esperar al watcher: el propio servicio lo ve al instante
	return nil
}

// IsRevoked indica si el jti fue revocado y el token todavía no venció.
func (d *Denylist) IsRevoked(jti string) bool {
	d.mu.RLock()
	exp, ok := d.revoked[jti]
	d.mu.RUnlock()
	return ok && time.Now().Before(exp)
}

// OnRevoke registra fn para que se llame cada vez que llega una revocación nueva
// (un jti que no estaba en la lista y todavía no venció). Sirve para cortar lo que
// no pasa por IsRevoked en cada request, como una conexión WebSocket ya abierta.
// fn corre en el goroutine del watcher: tiene que ser rápida y no bloquear.
// Las revocaciones cargadas al arrancar (en New) no disparan fn.
func (d *Denylist) OnRevoke(fn func(jti string)) {
	d.mu.Lock()
	d.listeners = append(d.listeners, fn)
	d.mu.Unlock()
}

// Stop detiene el watcher.
func (d *Denylist) Stop() {
	close(d.stop)
	_ = d.watcher.Stop()
}

func (d *Denylist) run() {
	prune := time.NewTicker(pruneInterval)
	defer prune.Stop()
	for {
		select {
		case <-d.stop:
			return
		case entry, ok := <-d.watcher.Updates():
			if !ok {
				return
			}
			if entry != nil {
				d.apply(entry)
			}
		case <-prune.C:
			d.prune()
		}
	}
}

func (d *Denylist) apply(entry nats.KeyValueEntry) {
	exp, err := time.Parse(time.RFC3339, string(entry.Value()))
	if err != nil {
		slog.Warn("revocation: invalid entry", "jti", entry.Key(), "err", err)
		return
	}
	d.set(entry.Key(), exp)
}

func (d *Denylist) set(jti string, exp time.Time) {
	d.mu.Lock()
	_, known := d.revoked[jti]
	d.revoked[jti] = exp
	listeners := d.listeners
	d.mu.Unlock()

	// Revoke y el watcher pueden traer el mismo jti: se avisa una sola vez.
	if known || !time.Now().Before(exp) {
		return
	}
	for _, fn := range listeners {
		fn(jti)
	}
}

// prune saca de memoria los tokens ya vencidos (el bucket los descarta por TTL).
func (d *Denylist) prune() {
	now := time.Now()
	d.mu.Lock()
	for jti, exp := range d.revoked {
		if now.After(exp) {
			delete(d.revoked, jti)
		}
	}
	d.mu.Unlock()
}

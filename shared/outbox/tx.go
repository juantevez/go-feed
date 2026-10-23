// Package outbox implementa el transactional outbox pattern sobre Postgres + NATS JetStream.
//
// Flujo:
//  1. El use case corre dentro de TxManager.WithinTx.
//  2. Los repositorios usan Conn(ctx, pool): si hay una tx en el ctx, la usan.
//  3. Publisher.Publish inserta el evento en la tabla outbox dentro de esa misma tx.
//     O se persisten el cambio de negocio y el evento, o ninguno.
//  4. Relay lee la tabla outbox y publica a JetStream (at-least-once, en orden).
package outbox

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier es el subconjunto común de *pgxpool.Pool y pgx.Tx que usan los repositorios.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type txKey struct{}

// ErrNoTx se retorna cuando una operación que requiere tx se ejecuta fuera de WithinTx.
var ErrNoTx = errors.New("outbox: no transaction in context")

// TxManager abre transacciones y las propaga por el context.
type TxManager struct {
	pool *pgxpool.Pool
}

func NewTxManager(pool *pgxpool.Pool) *TxManager {
	return &TxManager{pool: pool}
}

// WithinTx ejecuta fn dentro de una transacción. Si fn retorna error se hace
// rollback; si no, commit. Llamadas anidadas reutilizan la tx existente.
func (m *TxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := txFromContext(ctx); ok {
		return fn(ctx)
	}

	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("outbox: begin tx: %w", err)
	}
	// Rollback después de un Commit exitoso es un no-op.
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("outbox: commit tx: %w", err)
	}
	return nil
}

// Conn retorna la tx del context si existe; si no, el pool.
func Conn(ctx context.Context, pool *pgxpool.Pool) Querier {
	if tx, ok := txFromContext(ctx); ok {
		return tx
	}
	return pool
}

func txFromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	return tx, ok
}

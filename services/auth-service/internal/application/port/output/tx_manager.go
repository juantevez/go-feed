package output

import "context"

// TxManager es el puerto driven para ejecutar un use case dentro de una transacción.
// Todo lo que use el ctx recibido en fn (repositorios y EventPublisher) participa de la misma tx.
type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

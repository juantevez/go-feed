package input

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// FanOutCommand es el comando que dispara la propagación de un nuevo post
// al feed de todos los seguidores del autor.
type FanOutCommand struct {
	PostID      uuid.UUID
	AuthorID    uuid.UUID
	PublishedAt time.Time
}

// FanOutUseCase es el puerto driving para el worker de fan-out.
// El consumer NATS llama a este puerto al recibir posts.created.v1.
type FanOutUseCase interface {
	FanOut(ctx context.Context, cmd FanOutCommand) error
}

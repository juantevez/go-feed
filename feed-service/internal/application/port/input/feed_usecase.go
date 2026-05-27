package input

import (
	"context"

	"github.com/google/uuid"
	"github.com/juantevez/my-ig/feed-service/internal/domain/feed"
)

// GetFeedQuery es la query de entrada para obtener el timeline de un usuario.
type GetFeedQuery struct {
	UserID uuid.UUID
	Cursor *feed.Cursor // nil = primera página
	Limit  int          // máximo de entradas; default 20, máximo 50
}

// GetFeedResult es la respuesta paginada del timeline.
type GetFeedResult struct {
	Entries    []*feed.FeedEntry
	NextCursor *feed.Cursor // nil si no hay más páginas
	HasMore    bool
}

// FeedUseCase es el puerto driving principal del feed-service.
// El HTTP handler depende de esta interfaz, nunca del use case concreto.
type FeedUseCase interface {
	GetFeed(ctx context.Context, q GetFeedQuery) (*GetFeedResult, error)
}

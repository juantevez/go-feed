package post

import (
	"context"

	"github.com/google/uuid"
)

// Repository es el puerto driven para persistencia de posts.
type Repository interface {
	Save(ctx context.Context, p *Post) error
	FindByID(ctx context.Context, id uuid.UUID) (*Post, error)
	Update(ctx context.Context, p *Post) error
	// FindByAuthor retorna los posts no borrados de un autor, más recientes primero.
	// Con onlyVisible solo incluye los públicos y publicados (ver Post.IsVisible).
	FindByAuthor(ctx context.Context, authorID uuid.UUID, onlyVisible bool, limit, offset int) ([]*Post, error)
}

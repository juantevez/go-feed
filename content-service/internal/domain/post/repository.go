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
	FindByAuthor(ctx context.Context, authorID uuid.UUID, limit, offset int) ([]*Post, error)
}

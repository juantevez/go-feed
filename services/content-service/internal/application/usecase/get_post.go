package usecase

import (
	"context"
	"fmt"

	"github.com/juantevez/my-ig/content-service/internal/application/port/input"
	"github.com/juantevez/my-ig/content-service/internal/domain/post"
)

// GetPost retorna un post por ID respetando visibilidad.
func (s *PostService) GetPost(ctx context.Context, q input.GetPostQuery) (*input.GetPostResult, error) {
	p, err := s.posts.FindByID(ctx, q.PostID)
	if err != nil {
		return nil, fmt.Errorf("get_post: %w", err)
	}

	// Un post borrado no existe para nadie, tampoco para el autor.
	if p.Status == post.StatusDeleted {
		return nil, post.ErrPostNotFound
	}

	// El autor puede ver su propio post aunque no sea público;
	// otros usuarios solo ven posts públicos y publicados.
	if p.AuthorID != q.RequestingUser && !p.IsVisible() {
		return nil, post.ErrPostNotFound
	}

	if err := s.presignPrivateMedia(ctx, p); err != nil {
		return nil, fmt.Errorf("get_post: %w", err)
	}
	return &input.GetPostResult{Post: p}, nil
}

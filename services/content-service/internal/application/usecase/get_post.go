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

	// El autor siempre puede ver su propio post.
	if p.AuthorID == q.RequestingUser {
		return &input.GetPostResult{Post: p}, nil
	}

	// Otros usuarios solo ven posts públicos y publicados.
	if !p.IsVisible() {
		return nil, post.ErrPostNotFound
	}

	return &input.GetPostResult{Post: p}, nil
}

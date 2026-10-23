package usecase

import (
	"context"
	"fmt"

	"github.com/juantevez/my-ig/content-service/internal/application/port/input"
	"github.com/juantevez/my-ig/content-service/internal/domain/post"
)

const (
	defaultListLimit = 20
	maxListLimit     = 50
)

// ListUserPosts retorna los posts de un autor (más recientes primero) respetando visibilidad.
func (s *PostService) ListUserPosts(ctx context.Context, q input.ListUserPostsQuery) (*input.ListUserPostsResult, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}
	offset := max(q.Offset, 0)

	posts, err := s.posts.FindByAuthor(ctx, q.AuthorID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list_user_posts: %w", err)
	}

	// El autor ve todos sus posts; el resto solo los públicos y publicados.
	if q.AuthorID != q.RequestingUser {
		visible := make([]*post.Post, 0, len(posts))
		for _, p := range posts {
			if p.IsVisible() {
				visible = append(visible, p)
			}
		}
		posts = visible
	}

	return &input.ListUserPostsResult{Posts: posts}, nil
}

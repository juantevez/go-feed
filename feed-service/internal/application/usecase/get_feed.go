package usecase

import (
	"context"
	"fmt"

	"github.com/juantevez/my-ig/feed-service/internal/application/port/input"
	"github.com/juantevez/my-ig/feed-service/internal/domain/feed"
)

const (
	defaultLimit = 20
	maxLimit     = 50
)

// GetFeedService implementa input.FeedUseCase.
type GetFeedService struct {
	feeds feed.Repository
}

func NewGetFeedService(feeds feed.Repository) *GetFeedService {
	return &GetFeedService{feeds: feeds}
}

// GetFeed retorna la página del timeline solicitada.
// Aplica límites defensivos y construye el cursor de la próxima página.
func (s *GetFeedService) GetFeed(ctx context.Context, q input.GetFeedQuery) (*input.GetFeedResult, error) {
	limit := clampLimit(q.Limit)

	// Pedimos limit+1 para saber si hay más páginas sin hacer un COUNT.
	entries, err := s.feeds.GetPage(ctx, q.UserID, q.Cursor, limit+1)
	if err != nil {
		return nil, fmt.Errorf("get_feed: query page: %w", err)
	}

	hasMore := len(entries) > limit
	if hasMore {
		entries = entries[:limit] // devolvemos exactamente `limit` entradas
	}

	var nextCursor *feed.Cursor
	if hasMore && len(entries) > 0 {
		last := entries[len(entries)-1]
		nextCursor = &feed.Cursor{
			Score:  last.Score,
			PostID: last.PostID,
		}
	}

	return &input.GetFeedResult{
		Entries:    entries,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func clampLimit(l int) int {
	if l <= 0 {
		return defaultLimit
	}
	if l > maxLimit {
		return maxLimit
	}
	return l
}

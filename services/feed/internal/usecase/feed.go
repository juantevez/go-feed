package usecase

import (
	"context"
	"fmt"
	"log"

	"feed-backend/services/feed/internal/domain"
)

type FeedUseCase struct {
	store domain.FeedStore
}

func NewFeedUseCase(store domain.FeedStore) *FeedUseCase {
	return &FeedUseCase{store: store}
}

type FeedRequest struct {
	UserID string
	Cursor string
	Limit  int
}

type FeedResponse struct {
	Entries    []domain.FeedEntry `json:"entries"`
	NextCursor *string            `json:"next_cursor"`
}

func (uc *FeedUseCase) GetFeed(ctx context.Context, req FeedRequest) (*FeedResponse, error) {
	if req.Limit <= 0 || req.Limit > 50 {
		req.Limit = 20
	}

	// 1. Obtener IDs de Redis
	ids, nextRaw, err := uc.store.GetFeedPostIDs(ctx, req.UserID, req.Cursor, req.Limit)
	if err != nil {
		return nil, fmt.Errorf("get post ids: %w", err)
	}

	// 2. Enriquecer con metadatos (DB)
	var entries []domain.FeedEntry
	for _, id := range ids {
		meta, err := uc.store.GetPostMeta(ctx, id)
		if err != nil {
			log.Printf("[feed] skip missing post %s: %v", id, err)
			continue
		}
		username, _ := uc.store.GetUsername(ctx, meta.AuthorID)

		entries = append(entries, domain.FeedEntry{
			PostID:    meta.ID,
			AuthorID:  meta.AuthorID,
			Username:  username,
			Caption:   meta.Caption,
			MediaURLs: meta.MediaURLs,
			Likes:     meta.Likes,
			Comments:  meta.Comments,
			CreatedAt: meta.CreatedAt,
			Cursor:    fmt.Sprintf("%s_%s", meta.CreatedAt, meta.ID),
		})
	}

	var nextCursor *string
	if nextRaw != "" {
		nextCursor = &nextRaw
	}

	return &FeedResponse{Entries: entries, NextCursor: nextCursor}, nil
}

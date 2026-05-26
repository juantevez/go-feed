package usecase

import (
	"context"
	"fmt"
	"log"
	"time"

	"feed-backend/pkg/eventbus"
	"feed-backend/services/feed/internal/domain"
)

type FanOutWorker struct {
	store domain.FeedStore
	bus   eventbus.Subscriber
}

func NewFanOutWorker(store domain.FeedStore, bus eventbus.Subscriber) *FanOutWorker {
	return &FanOutWorker{store: store, bus: bus}
}

func (w *FanOutWorker) Start(ctx context.Context) error {
	return w.bus.Subscribe(ctx, "posts.created.v1", "feed-workers", func(ctx context.Context, msg eventbus.Envelope) error {
		authorID, _ := msg.Payload["author_id"].(string)
		postID, _ := msg.Payload["post_id"].(string)
		createdAtStr, _ := msg.Payload["created_at"].(string)

		t, err := time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			t = time.Now().UTC()
		}
		score := float64(t.UnixMilli())

		// 1. Obtener seguidores activos
		followers, err := w.store.GetFollowers(ctx, authorID)
		if err != nil {
			return fmt.Errorf("get followers: %w", err)
		}

		// 2. Fan-out a Redis (pipeline implícito en push)
		for _, fid := range followers {
			if err := w.store.PushToFeed(ctx, fid, postID, score); err != nil {
				log.Printf("[fanout] push to %s failed: %v", fid, err)
			}
		}

		log.Printf("[fanout] post %s pushed to %d followers", postID, len(followers))
		return nil
	})
}

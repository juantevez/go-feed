package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/juantevez/my-ig/feed-service/internal/application/port/input"
	"github.com/juantevez/my-ig/feed-service/internal/domain/feed"
)

// fanOutBatchSize es el tamaño del batch para BulkInsert.
// Evita queries gigantes con miles de seguidores.
const fanOutBatchSize = 500

// FanOutService implementa input.FanOutUseCase.
// Consume posts.created.v1 y propaga el post al feed de cada seguidor activo.
type FanOutService struct {
	feeds     feed.Repository
	followers feed.FollowerRepository
}

func NewFanOutService(feeds feed.Repository, followers feed.FollowerRepository) *FanOutService {
	return &FanOutService{feeds: feeds, followers: followers}
}

// FanOut propaga el post a los feeds de todos los seguidores activos del autor.
//
// Estrategia: fan-out sincrónico en batches.
// Para autores con > 10k seguidores se usará fan-in lazy (fase 2).
//
// Idempotencia: BulkInsert usa ON CONFLICT DO NOTHING — si el worker
// reprocesa el mismo evento, no genera duplicados.
func (s *FanOutService) FanOut(ctx context.Context, cmd input.FanOutCommand) error {
	followerIDs, err := s.followers.GetActiveFollowers(ctx, cmd.AuthorID)
	if err != nil {
		return fmt.Errorf("fan_out: get followers: %w", err)
	}

	if len(followerIDs) == 0 {
		slog.Info("fan_out: no active followers, skipping",
			"author_id", cmd.AuthorID,
			"post_id", cmd.PostID,
		)
		return nil
	}

	slog.Info("fan_out: propagating post",
		"post_id", cmd.PostID,
		"author_id", cmd.AuthorID,
		"followers", len(followerIDs),
	)

	// Procesamos en batches para no saturar la DB con un INSERT masivo.
	for i := 0; i < len(followerIDs); i += fanOutBatchSize {
		end := min(i+fanOutBatchSize, len(followerIDs))
		batch := followerIDs[i:end]

		entries := make([]*feed.FeedEntry, 0, len(batch))
		for _, followerID := range batch {
			entry, err := feed.New(followerID, cmd.PostID, cmd.AuthorID, cmd.PublishedAt)
			if err != nil {
				// Log y continuar — un followerID inválido no debe detener el batch completo.
				slog.Warn("fan_out: skip invalid entry",
					"follower_id", followerID,
					"err", err,
				)
				continue
			}
			entries = append(entries, entry)
		}

		if len(entries) == 0 {
			continue
		}

		if err := s.feeds.BulkInsert(ctx, entries); err != nil {
			return fmt.Errorf("fan_out: bulk insert batch %d-%d: %w", i, end, err)
		}
	}

	slog.Info("fan_out: done", "post_id", cmd.PostID, "followers", len(followerIDs))
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

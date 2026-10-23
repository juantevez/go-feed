package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/juantevez/my-ig/feed-service/internal/application/port/input"
	"github.com/juantevez/my-ig/feed-service/internal/domain/feed"
)

// fanOutBatchSize es el tamaño del batch para BulkInsert.
// Evita queries gigantes con miles de seguidores.
const fanOutBatchSize = 500

// FanOutService implementa input.FanOutUseCase.
// Consume posts.created.v1 y propaga el post al feed del autor y de cada seguidor activo.
type FanOutService struct {
	feeds     feed.Repository
	followers feed.FollowerRepository
}

func NewFanOutService(feeds feed.Repository, followers feed.FollowerRepository) *FanOutService {
	return &FanOutService{feeds: feeds, followers: followers}
}

// FanOut propaga el post al feed del autor y a los de todos sus seguidores activos.
//
// Estrategia: fan-out sincrónico en batches.
// Para autores con > 10k seguidores se usará fan-in lazy (fase 2).
//
// Idempotencia: BulkInsert usa ON CONFLICT DO NOTHING — si el worker
// reprocesa el mismo evento, no genera duplicados.
func (s *FanOutService) FanOut(ctx context.Context, cmd input.FanOutCommand) error {
	// Índice por autor: lo usa el backfill cuando alguien empieza a seguirlo.
	if err := s.feeds.SaveAuthorPost(ctx, cmd.PostID, cmd.AuthorID, cmd.PublishedAt); err != nil {
		return fmt.Errorf("fan_out: save author post: %w", err)
	}

	followerIDs, err := s.followers.GetActiveFollowers(ctx, cmd.AuthorID)
	if err != nil {
		return fmt.Errorf("fan_out: get followers: %w", err)
	}

	// El autor siempre ve sus propios posts en su timeline.
	recipients := make([]uuid.UUID, 0, len(followerIDs)+1)
	recipients = append(recipients, cmd.AuthorID)
	for _, id := range followerIDs {
		if id != cmd.AuthorID {
			recipients = append(recipients, id)
		}
	}

	slog.Info("fan_out: propagating post",
		"post_id", cmd.PostID,
		"author_id", cmd.AuthorID,
		"followers", len(followerIDs),
	)

	// Procesamos en batches para no saturar la DB con un INSERT masivo.
	for i := 0; i < len(recipients); i += fanOutBatchSize {
		end := min(i+fanOutBatchSize, len(recipients))
		batch := recipients[i:end]

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

	slog.Info("fan_out: done", "post_id", cmd.PostID, "recipients", len(recipients))
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

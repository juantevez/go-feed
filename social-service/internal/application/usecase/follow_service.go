package usecase

import (
	"context"
	"fmt"
	"log/slog"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/juantevez/my-ig/shared/events"
	"github.com/juantevez/my-ig/social-service/internal/application/port/input"
	"github.com/juantevez/my-ig/social-service/internal/application/port/output"
	"github.com/juantevez/my-ig/social-service/internal/domain/follow"
)

// FollowService implementa input.FollowUseCase.
type FollowService struct {
	follows   follow.Repository
	publisher output.EventPublisher
}

func NewFollowService(follows follow.Repository, publisher output.EventPublisher) *FollowService {
	return &FollowService{follows: follows, publisher: publisher}
}

// Follow crea la relación follower → following.
// Detecta si la relación es mutua y actualiza ambos lados.
// Publica users.followed.v1 al bus (fire-and-forget).
func (s *FollowService) Follow(ctx context.Context, cmd input.FollowCommand) error {
	// Validar en el dominio (auto-follow, UUIDs nulos)
	f, err := follow.New(cmd.FollowerID, cmd.FollowingID)
	if err != nil {
		return fmt.Errorf("follow: %w", err)
	}

	// Persistir la relación
	if err := s.follows.Save(ctx, f); err != nil {
		return fmt.Errorf("follow: save: %w", err)
	}

	// Detectar mutualidad: ¿followingID ya sigue a followerID?
	mutual, err := s.follows.Exists(ctx, cmd.FollowingID, cmd.FollowerID)
	if err == nil && mutual {
		f.Mutual = true
		if err := s.follows.UpdateMutual(ctx, cmd.FollowerID, cmd.FollowingID, true); err != nil {
			slog.Warn("follow: update mutual failed", "err", err)
		}
	}

	traceID := chimiddleware.GetReqID(ctx)
	s.publishAsync(events.TopicUserFollowed, follow.NewFollowedEnvelope(f, traceID))

	return nil
}

// Unfollow elimina la relación follower → following.
// Si era mutua, actualiza el flag en la relación inversa.
// Publica users.unfollowed.v1 al bus (fire-and-forget).
func (s *FollowService) Unfollow(ctx context.Context, cmd input.UnfollowCommand) error {
	if err := s.follows.Delete(ctx, cmd.FollowerID, cmd.FollowingID); err != nil {
		return fmt.Errorf("unfollow: %w", err)
	}

	// Si era mutua, limpiar el flag en la dirección inversa
	if err := s.follows.UpdateMutual(ctx, cmd.FollowingID, cmd.FollowerID, false); err != nil {
		slog.Warn("unfollow: clear mutual failed", "err", err)
	}

	traceID := chimiddleware.GetReqID(ctx)
	s.publishAsync(
		events.TopicUserUnfollowed,
		follow.NewUnfollowedEnvelope(cmd.FollowerID, cmd.FollowingID, traceID),
	)

	return nil
}

// GetFollowers retorna los seguidores de un usuario.
func (s *FollowService) GetFollowers(ctx context.Context, q input.GetFollowersQuery) (*input.FollowListResult, error) {
	limit := clampLimit(q.Limit)
	follows, err := s.follows.GetFollowers(ctx, q.UserID, limit, q.Offset)
	if err != nil {
		return nil, fmt.Errorf("get_followers: %w", err)
	}
	return &input.FollowListResult{Follows: follows}, nil
}

// GetFollowing retorna los usuarios que sigue un usuario.
func (s *FollowService) GetFollowing(ctx context.Context, q input.GetFollowingQuery) (*input.FollowListResult, error) {
	limit := clampLimit(q.Limit)
	follows, err := s.follows.GetFollowing(ctx, q.UserID, limit, q.Offset)
	if err != nil {
		return nil, fmt.Errorf("get_following: %w", err)
	}
	return &input.FollowListResult{Follows: follows}, nil
}

// GetStats retorna los contadores de followers/following de un usuario.
func (s *FollowService) GetStats(ctx context.Context, userID uuid.UUID) (*follow.Stats, error) {
	stats, err := s.follows.GetStats(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get_stats: %w", err)
	}
	return stats, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (s *FollowService) publishAsync(topic string, envelope events.Envelope) {
	go func() {
		if err := s.publisher.Publish(context.Background(), topic, envelope); err != nil {
			slog.Error("event publish failed",
				"topic", topic,
				"event_id", envelope.EventID,
				"err", err,
			)
		}
	}()
}

func clampLimit(l int) int {
	if l <= 0 {
		return 20
	}
	if l > 100 {
		return 100
	}
	return l
}

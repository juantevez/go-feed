package usecase

import (
	"context"
	"fmt"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/juantevez/my-ig/shared/events"
	"github.com/juantevez/my-ig/social-service/internal/application/port/input"
	"github.com/juantevez/my-ig/social-service/internal/application/port/output"
	"github.com/juantevez/my-ig/social-service/internal/domain/follow"
	"github.com/juantevez/my-ig/social-service/internal/domain/user"
)

// FollowService implementa input.FollowUseCase.
type FollowService struct {
	tx        output.TxManager
	follows   follow.Repository
	users     user.Repository
	publisher output.EventPublisher
}

func NewFollowService(tx output.TxManager, follows follow.Repository, users user.Repository, publisher output.EventPublisher) *FollowService {
	return &FollowService{tx: tx, follows: follows, users: users, publisher: publisher}
}

// Follow crea la relación follower → following.
// Detecta si la relación es mutua y actualiza ambos lados.
// La relación y el evento users.followed.v1 se persisten en la misma tx (outbox).
func (s *FollowService) Follow(ctx context.Context, cmd input.FollowCommand) error {
	// Validar en el dominio (auto-follow, UUIDs nulos)
	f, err := follow.New(cmd.FollowerID, cmd.FollowingID)
	if err != nil {
		return fmt.Errorf("follow: %w", err)
	}

	// El seguido tiene que existir. El seguidor no hace falta chequearlo: es el
	// sub de un token emitido por auth-service.
	exists, err := s.users.Exists(ctx, cmd.FollowingID)
	if err != nil {
		return fmt.Errorf("follow: check user: %w", err)
	}
	if !exists {
		return fmt.Errorf("follow: %w", follow.ErrUserNotFound)
	}

	traceID := chimiddleware.GetReqID(ctx)

	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		// Con el par lockeado, un follow cruzado simultáneo ve esta relación.
		if err := s.follows.LockPair(ctx, cmd.FollowerID, cmd.FollowingID); err != nil {
			return fmt.Errorf("follow: lock: %w", err)
		}

		// Persistir la relación
		if err := s.follows.Save(ctx, f); err != nil {
			return fmt.Errorf("follow: save: %w", err)
		}

		// Detectar mutualidad: ¿followingID ya sigue a followerID?
		mutual, err := s.follows.Exists(ctx, cmd.FollowingID, cmd.FollowerID)
		if err != nil {
			return fmt.Errorf("follow: check mutual: %w", err)
		}
		if mutual {
			f.Mutual = true
			if err := s.follows.UpdateMutual(ctx, cmd.FollowerID, cmd.FollowingID, true); err != nil {
				return fmt.Errorf("follow: update mutual: %w", err)
			}
		}

		if err := s.publisher.Publish(ctx, events.TopicUserFollowed, follow.NewFollowedEnvelope(f, traceID)); err != nil {
			return fmt.Errorf("follow: enqueue event: %w", err)
		}
		return nil
	})
}

// Unfollow elimina la relación follower → following.
// Si era mutua, actualiza el flag en la relación inversa.
// La baja y el evento users.unfollowed.v1 se persisten en la misma tx (outbox).
func (s *FollowService) Unfollow(ctx context.Context, cmd input.UnfollowCommand) error {
	traceID := chimiddleware.GetReqID(ctx)

	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.follows.LockPair(ctx, cmd.FollowerID, cmd.FollowingID); err != nil {
			return fmt.Errorf("unfollow: lock: %w", err)
		}
		if err := s.follows.Delete(ctx, cmd.FollowerID, cmd.FollowingID); err != nil {
			return fmt.Errorf("unfollow: %w", err)
		}

		// Si era mutua, limpiar el flag en la dirección inversa
		if err := s.follows.UpdateMutual(ctx, cmd.FollowingID, cmd.FollowerID, false); err != nil {
			return fmt.Errorf("unfollow: clear mutual: %w", err)
		}

		envelope := follow.NewUnfollowedEnvelope(cmd.FollowerID, cmd.FollowingID, traceID)
		if err := s.publisher.Publish(ctx, events.TopicUserUnfollowed, envelope); err != nil {
			return fmt.Errorf("unfollow: enqueue event: %w", err)
		}
		return nil
	})
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

func clampLimit(l int) int {
	if l <= 0 {
		return 20
	}
	if l > 100 {
		return 100
	}
	return l
}

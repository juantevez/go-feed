package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/juantevez/my-ig/notification-service/internal/application/port/input"
	"github.com/juantevez/my-ig/notification-service/internal/application/port/output"
	"github.com/juantevez/my-ig/notification-service/internal/domain/notification"
)

// NotificationService implementa input.NotificationUseCase.
type NotificationService struct {
	tx      output.TxManager
	repo    notification.Repository
	follows notification.FollowStateRepository
	pusher  output.NotificationPusher
}

func NewNotificationService(
	tx output.TxManager,
	repo notification.Repository,
	follows notification.FollowStateRepository,
	pusher output.NotificationPusher,
) *NotificationService {
	return &NotificationService{tx: tx, repo: repo, follows: follows, pusher: pusher}
}

// GetNotifications retorna las notificaciones de un usuario con conteo de no leídas.
func (s *NotificationService) GetNotifications(ctx context.Context, q input.GetNotificationsQuery) (*input.GetNotificationsResult, error) {
	limit := clampLimit(q.Limit)
	offset := max(q.Offset, 0) // Postgres rechaza un OFFSET negativo

	notifications, err := s.repo.GetByUser(ctx, q.UserID, limit, offset, q.UnreadOnly)
	if err != nil {
		return nil, fmt.Errorf("get_notifications: %w", err)
	}

	unread, err := s.repo.CountUnread(ctx, q.UserID)
	if err != nil {
		return nil, fmt.Errorf("get_notifications: count unread: %w", err)
	}

	return &input.GetNotificationsResult{
		Notifications: notifications,
		UnreadCount:   unread,
	}, nil
}

// MarkRead marca una notificación como leída. Idempotente: marcar una ya leída
// no es un error.
func (s *NotificationService) MarkRead(ctx context.Context, notificationID, userID uuid.UUID) error {
	if err := s.repo.MarkRead(ctx, notificationID, userID); err != nil {
		return fmt.Errorf("mark_read: %w", err)
	}
	return nil
}

// MarkAllRead marca todas las notificaciones de un usuario como leídas.
func (s *NotificationService) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	if err := s.repo.MarkAllRead(ctx, userID); err != nil {
		return fmt.Errorf("mark_all_read: %w", err)
	}
	return nil
}

// NotifyNewFollower aplica un users.followed.v1 ocurrido en followedAt: guarda la
// notificación new_follower (n) y la pushea por WebSocket.
//
// Hay como mucho una notificación por par seguidor/seguido: si quedaba una de un
// follow anterior, se reemplaza. Si ya se procesó un evento más nuevo de esa
// relación (ej. el unfollow llegó antes), no hace nada. Idempotente: una
// reentrega del mismo evento no vuelve a guardar ni a pushear.
func (s *NotificationService) NotifyNewFollower(ctx context.Context, n *notification.Notification, followedAt time.Time) error {
	var created bool
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		applied, err := s.follows.ApplyFollow(ctx, n.SourceUserID, n.TargetUserID, followedAt)
		if err != nil {
			return fmt.Errorf("apply follow: %w", err)
		}
		if !applied {
			slog.InfoContext(ctx, "notify_new_follower: stale or duplicate follow ignored",
				"event_id", n.EventID, "target_user_id", n.TargetUserID, "follower_id", n.SourceUserID)
			return nil
		}
		if _, err := s.repo.DeleteNewFollower(ctx, n.TargetUserID, n.SourceUserID); err != nil {
			return fmt.Errorf("delete previous: %w", err)
		}
		created, err = s.repo.Save(ctx, n)
		if err != nil {
			return fmt.Errorf("save: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("notify_new_follower: %w", err)
	}

	// Recién después del commit: si la tx falla, no hay que avisar nada.
	if created {
		s.push(n)
	}
	return nil
}

// RemoveNewFollower aplica un users.unfollowed.v1 ocurrido en unfollowedAt:
// borra la notificación new_follower de ese seguidor. Si ya se procesó un evento
// más nuevo de esa relación (un re-follow), no hace nada.
func (s *NotificationService) RemoveNewFollower(ctx context.Context, followerID, followingID uuid.UUID, unfollowedAt time.Time) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		applied, err := s.follows.ApplyUnfollow(ctx, followerID, followingID, unfollowedAt)
		if err != nil {
			return fmt.Errorf("apply unfollow: %w", err)
		}
		if !applied {
			slog.InfoContext(ctx, "remove_new_follower: stale or duplicate unfollow ignored",
				"target_user_id", followingID, "follower_id", followerID)
			return nil
		}
		n, err := s.repo.DeleteNewFollower(ctx, followingID, followerID)
		if err != nil {
			return fmt.Errorf("delete: %w", err)
		}
		slog.InfoContext(ctx, "remove_new_follower: done", "target_user_id", followingID, "follower_id", followerID, "deleted", n)
		return nil
	})
	if err != nil {
		return fmt.Errorf("remove_new_follower: %w", err)
	}
	return nil
}

// push envía la notificación por WebSocket sin bloquear: si el usuario no está
// conectado no pasa nada, y la notificación igual está en la DB.
func (s *NotificationService) push(n *notification.Notification) {
	go func() {
		_ = s.pusher.Push(context.Background(), n.TargetUserID, n)
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

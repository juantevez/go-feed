package usecase

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/juantevez/my-ig/notification-service/internal/application/port/input"
	"github.com/juantevez/my-ig/notification-service/internal/application/port/output"
	"github.com/juantevez/my-ig/notification-service/internal/domain/notification"
)

// NotificationService implementa input.NotificationUseCase.
type NotificationService struct {
	repo   notification.Repository
	pusher output.NotificationPusher
}

func NewNotificationService(
	repo notification.Repository,
	pusher output.NotificationPusher,
) *NotificationService {
	return &NotificationService{repo: repo, pusher: pusher}
}

// GetNotifications retorna las notificaciones de un usuario con conteo de no leídas.
func (s *NotificationService) GetNotifications(ctx context.Context, q input.GetNotificationsQuery) (*input.GetNotificationsResult, error) {
	limit := clampLimit(q.Limit)

	notifications, err := s.repo.GetByUser(ctx, q.UserID, limit, q.Offset, q.UnreadOnly)
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

// MarkRead marca una notificación como leída.
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

// CreateAndPush crea una notificación, la persiste y la envía via WebSocket.
// Llamado internamente por los workers de NATS.
func (s *NotificationService) CreateAndPush(ctx context.Context, n *notification.Notification) error {
	if err := s.repo.Save(ctx, n); err != nil {
		return fmt.Errorf("create_and_push: save: %w", err)
	}

	// Push via WebSocket — fire-and-forget si el usuario no está conectado.
	go func() {
		_ = s.pusher.Push(context.Background(), n.TargetUserID, n)
	}()

	return nil
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

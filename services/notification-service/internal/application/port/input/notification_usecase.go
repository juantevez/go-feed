package input

import (
	"context"

	"github.com/google/uuid"
	"github.com/juantevez/my-ig/notification-service/internal/domain/notification"
)

// GetNotificationsQuery es la query para listar notificaciones de un usuario.
type GetNotificationsQuery struct {
	UserID     uuid.UUID
	Limit      int
	Offset     int
	UnreadOnly bool
}

// GetNotificationsResult es la respuesta paginada.
type GetNotificationsResult struct {
	Notifications []*notification.Notification
	UnreadCount   int
}

// NotificationUseCase es el puerto driving del notification-service.
type NotificationUseCase interface {
	GetNotifications(ctx context.Context, q GetNotificationsQuery) (*GetNotificationsResult, error)
	MarkRead(ctx context.Context, notificationID, userID uuid.UUID) error
	MarkAllRead(ctx context.Context, userID uuid.UUID) error
}

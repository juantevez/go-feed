package notification

import (
	"context"

	"github.com/google/uuid"
)

// Repository es el puerto driven para persistencia de notificaciones.
type Repository interface {
	// Save persiste una nueva notificación.
	Save(ctx context.Context, n *Notification) error

	// GetByUser retorna las notificaciones de un usuario (más recientes primero).
	// unreadOnly filtra solo las no leídas.
	GetByUser(ctx context.Context, userID uuid.UUID, limit, offset int, unreadOnly bool) ([]*Notification, error)

	// MarkRead marca una notificación como leída.
	// Retorna ErrNotOwner si userID no es el destinatario.
	MarkRead(ctx context.Context, notificationID, userID uuid.UUID) error

	// MarkAllRead marca todas las notificaciones de un usuario como leídas.
	MarkAllRead(ctx context.Context, userID uuid.UUID) error

	// CountUnread retorna el número de notificaciones no leídas de un usuario.
	CountUnread(ctx context.Context, userID uuid.UUID) (int, error)
}

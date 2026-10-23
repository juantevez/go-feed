package notification

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository es el puerto driven para persistencia de notificaciones.
type Repository interface {
	// Save persiste una nueva notificación. Si ya existe una para el mismo
	// (EventID, TargetUserID) no hace nada y retorna false (reentrega del evento).
	Save(ctx context.Context, n *Notification) (bool, error)

	// GetByUser retorna las notificaciones de un usuario (más recientes primero).
	// unreadOnly filtra solo las no leídas.
	GetByUser(ctx context.Context, userID uuid.UUID, limit, offset int, unreadOnly bool) ([]*Notification, error)

	// MarkRead marca una notificación como leída. Idempotente: si ya estaba
	// leída no hace nada. Retorna ErrNotificationNotFound si no existe o si
	// userID no es el destinatario.
	MarkRead(ctx context.Context, notificationID, userID uuid.UUID) error

	// MarkAllRead marca todas las notificaciones de un usuario como leídas.
	MarkAllRead(ctx context.Context, userID uuid.UUID) error

	// CountUnread retorna el número de notificaciones no leídas de un usuario.
	CountUnread(ctx context.Context, userID uuid.UUID) (int, error)

	// DeleteNewFollower borra las notificaciones new_follower de followerID para
	// targetUserID. Retorna cuántas borró.
	DeleteNewFollower(ctx context.Context, targetUserID, followerID uuid.UUID) (int64, error)
}

// FollowStateRepository guarda el último estado conocido de cada relación de
// follow, con last-write-wins por timestamp del evento: users.followed y
// users.unfollowed llegan por subscriptions separadas y pueden desordenarse.
type FollowStateRepository interface {
	// ApplyFollow registra el follow ocurrido en at. Retorna false si ya hay un
	// evento igual o más nuevo para la relación (el follow está viejo).
	ApplyFollow(ctx context.Context, followerID, followingID uuid.UUID, at time.Time) (bool, error)

	// ApplyUnfollow registra el unfollow ocurrido en at. Retorna false si ya hay
	// un evento igual o más nuevo para la relación.
	ApplyUnfollow(ctx context.Context, followerID, followingID uuid.UUID, at time.Time) (bool, error)
}

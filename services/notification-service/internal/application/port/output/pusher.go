package output

import (
	"context"

	"github.com/google/uuid"
	"github.com/juantevez/my-ig/notification-service/internal/domain/notification"
)

// NotificationPusher es el puerto driven para enviar notificaciones
// en tiempo real a clientes conectados via WebSocket.
type NotificationPusher interface {
	// Push envía una notificación al usuario si está conectado.
	// Si el usuario no tiene conexión activa, retorna sin error (fire-and-forget).
	Push(ctx context.Context, userID uuid.UUID, n *notification.Notification) error
}

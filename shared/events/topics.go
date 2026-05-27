package events

// Tópicos NATS del sistema.
// Convención: {dominio}.{entidad}.{acción}.{versión}
//
// Regla funcional: un tópico nunca contiene más de un tipo semántico.
// Si cambia la semántica del payload, se incrementa la versión
// y se crea un nuevo tópico — nunca se modifica el existente.

// ── Auth Service ─────────────────────────────────────────────────────────────

const (
	// TopicAuthUserRegistered se publica cuando un usuario completa el registro.
	// Producer: auth-service | Consumers: feed-service, notification-service
	TopicAuthUserRegistered = "auth.user.registered.v1"

	// TopicAuthUserLoggedIn se publica en cada login exitoso.
	// Producer: auth-service | Consumers: analytics (fase 2)
	TopicAuthUserLoggedIn = "auth.user.logged_in.v1"
)

// ── Content Service (fase 2) ──────────────────────────────────────────────────

const (
	// TopicPostCreated se publica cuando un post es persistido y visible.
	// Producer: content-service | Consumers: feed-service, notification-service, analytics
	TopicPostCreated = "posts.created.v1"

	// TopicPostDeleted se publica en hard delete o moderación.
	// Producer: content-service | Consumers: feed-service (invalidar caché)
	TopicPostDeleted = "posts.deleted.v1"
)

// ── Social Service (fase 2) ───────────────────────────────────────────────────

const (
	// TopicUserFollowed se publica cuando un usuario sigue a otro.
	// Producer: social-service | Consumers: feed-service (rebuild parcial), notification-service
	TopicUserFollowed = "users.followed.v1"

	// TopicUserUnfollowed se publica en unfollow.
	// Producer: social-service | Consumers: feed-service (ZREM posts del autor)
	TopicUserUnfollowed = "users.unfollowed.v1"
)

// ── Interaction Service (fase 2) ──────────────────────────────────────────────

const (
	// TopicPostLiked se publica en like/unlike (campo action: "add"|"remove").
	// Producer: interaction-service | Consumers: feed-service (score), notification-service
	TopicPostLiked = "interactions.liked.v1"

	// TopicCommentAdded se publica cuando se agrega un comentario.
	// Producer: interaction-service | Consumers: notification-service
	TopicCommentAdded = "interactions.comment_added.v1"
)

// ── Dead Letter Queues ────────────────────────────────────────────────────────

// DLQPrefix es el prefijo para los tópicos de dead letter queue.
// Ejemplo: "dlq.posts.created.v1"
const DLQPrefix = "dlq."

// DLQ retorna el tópico de DLQ para un tópico dado.
func DLQ(topic string) string {
	return DLQPrefix + topic
}

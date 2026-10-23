package feed

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository es el puerto driven (secundario) para persistencia del feed.
// El adaptador Postgres implementa esta interfaz.
type Repository interface {
	// LockPost serializa, hasta el fin de la tx, el procesamiento de posts.created
	// y posts.deleted de un mismo post. Debe llamarse dentro de una tx.
	LockPost(ctx context.Context, postID uuid.UUID) error

	// MarkPostDeleted registra el tombstone del post (deleted_posts). Idempotente.
	MarkPostDeleted(ctx context.Context, postID uuid.UUID, deletedAt time.Time) error

	// IsPostDeleted indica si el post tiene tombstone: un posts.created que llega
	// después de su posts.deleted no debe volver a meterlo en los feeds.
	IsPostDeleted(ctx context.Context, postID uuid.UUID) (bool, error)

	// SaveAuthorPost registra el post en el índice local de posts por autor
	// (author_posts). Idempotente.
	SaveAuthorPost(ctx context.Context, postID, authorID uuid.UUID, publishedAt time.Time) error

	// InsertForFollowers agrega el post al feed de todos los que siguen al autor
	// (relación activa), en una sola operación y sin tope de seguidores. No incluye
	// al autor. Ignora duplicados. Retorna cuántas entradas insertó.
	InsertForFollowers(ctx context.Context, postID, authorID uuid.UUID, publishedAt time.Time) (int64, error)

	// BulkInsert inserta múltiples FeedEntry en una sola operación.
	// Usado por el fan-out worker al propagar un nuevo post.
	// Ignora duplicados (ON CONFLICT DO NOTHING) para idempotencia.
	BulkInsert(ctx context.Context, entries []*FeedEntry) error

	// GetPage retorna una página del feed de un usuario usando cursor-based pagination.
	// Si cursor es nil retorna desde el inicio (más recientes primero).
	// Siempre retorna como máximo `limit` entradas.
	GetPage(ctx context.Context, userID uuid.UUID, cursor *Cursor, limit int) ([]*FeedEntry, error)

	// DeleteByPostID elimina todas las entradas de feed asociadas a un post.
	// Usado cuando un post es eliminado (invalidación).
	DeleteByPostID(ctx context.Context, postID uuid.UUID) error

	// DeleteByAuthorFromUser elimina las entradas de un autor específico
	// del feed de un usuario (usado en unfollow).
	DeleteByAuthorFromUser(ctx context.Context, userID, authorID uuid.UUID) error
}

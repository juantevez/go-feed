package feed

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository es el puerto driven (secundario) para persistencia del feed.
// El adaptador Postgres implementa esta interfaz.
type Repository interface {
	// SaveAuthorPost registra el post en el índice local de posts por autor
	// (author_posts). Idempotente.
	SaveAuthorPost(ctx context.Context, postID, authorID uuid.UUID, publishedAt time.Time) error

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

// FollowerRepository es el puerto para consultar el grafo social.
// El feed-service no gestiona follows — los consulta como read-only.
type FollowerRepository interface {
	// GetActiveFollowers retorna los IDs de seguidores activos de un autor.
	// "Activos" = que han hecho login en los últimos 30 días (fan-out selectivo).
	GetActiveFollowers(ctx context.Context, authorID uuid.UUID) ([]uuid.UUID, error)
}

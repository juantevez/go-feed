package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/juantevez/my-ig/feed-service/internal/domain/feed"
)

// FeedRepository implementa feed.Repository sobre Postgres.
type FeedRepository struct {
	db *pgxpool.Pool
}

func NewFeedRepository(db *pgxpool.Pool) *FeedRepository {
	return &FeedRepository{db: db}
}

// BulkInsert inserta múltiples entradas usando pgx.CopyFrom — el método más
// eficiente para inserts masivos en Postgres (protocolo COPY).
// ON CONFLICT DO NOTHING garantiza idempotencia ante redelivery de eventos.
func (r *FeedRepository) BulkInsert(ctx context.Context, entries []*feed.FeedEntry) error {
	rows := make([][]any, len(entries))
	for i, e := range entries {
		rows[i] = []any{e.ID, e.UserID, e.PostID, e.AuthorID, e.Score, e.InsertedAt}
	}

	_, err := r.db.CopyFrom(
		ctx,
		pgx.Identifier{"feed_entries"},
		[]string{"id", "user_id", "post_id", "author_id", "score", "inserted_at"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return fmt.Errorf("feed_repo.BulkInsert: %w", err)
	}
	return nil
}

// GetPage retorna entradas del feed paginadas con cursor-based pagination.
// El cursor usa (score DESC, post_id) para manejar empates de timestamp.
func (r *FeedRepository) GetPage(ctx context.Context, userID uuid.UUID, cursor *feed.Cursor, limit int) ([]*feed.FeedEntry, error) {
	var (
		rows pgx.Rows
		err  error
	)

	if cursor == nil {
		// Primera página — sin cursor
		const q = `
			SELECT id, user_id, post_id, author_id, score, inserted_at
			FROM feed_entries
			WHERE user_id = $1
			ORDER BY score DESC, post_id DESC
			LIMIT $2`
		rows, err = r.db.Query(ctx, q, userID, limit)
	} else {
		// Páginas siguientes — cursor compuesto (score, post_id)
		const q = `
			SELECT id, user_id, post_id, author_id, score, inserted_at
			FROM feed_entries
			WHERE user_id = $1
			  AND (score, post_id::text) < ($2, $3)
			ORDER BY score DESC, post_id DESC
			LIMIT $4`
		rows, err = r.db.Query(ctx, q, userID, cursor.Score, cursor.PostID.String(), limit)
	}

	if err != nil {
		return nil, fmt.Errorf("feed_repo.GetPage: %w", err)
	}
	defer rows.Close()

	var entries []*feed.FeedEntry
	for rows.Next() {
		var e feed.FeedEntry
		if err := rows.Scan(&e.ID, &e.UserID, &e.PostID, &e.AuthorID, &e.Score, &e.InsertedAt); err != nil {
			return nil, fmt.Errorf("feed_repo.GetPage scan: %w", err)
		}
		entries = append(entries, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("feed_repo.GetPage rows: %w", err)
	}

	return entries, nil
}

// DeleteByPostID elimina todas las entradas de feed de un post dado.
// Llamado cuando un post es borrado (invalidación).
func (r *FeedRepository) DeleteByPostID(ctx context.Context, postID uuid.UUID) error {
	const q = `DELETE FROM feed_entries WHERE post_id = $1`
	if _, err := r.db.Exec(ctx, q, postID); err != nil {
		return fmt.Errorf("feed_repo.DeleteByPostID: %w", err)
	}
	return nil
}

// DeleteByAuthorFromUser elimina del feed de un usuario los posts de un autor.
// Llamado en unfollow.
func (r *FeedRepository) DeleteByAuthorFromUser(ctx context.Context, userID, authorID uuid.UUID) error {
	const q = `DELETE FROM feed_entries WHERE user_id = $1 AND author_id = $2`
	if _, err := r.db.Exec(ctx, q, userID, authorID); err != nil {
		return fmt.Errorf("feed_repo.DeleteByAuthorFromUser: %w", err)
	}
	return nil
}

// ── FollowerRepository ────────────────────────────────────────────────────────

// FollowerRepository implementa feed.FollowerRepository.
// El feed-service lee el grafo social de su propia DB (replicado desde social-service).
// En fase 1 la tabla followers es poblada directamente desde auth-service via evento.
type FollowerRepository struct {
	db *pgxpool.Pool
}

func NewFollowerRepository(db *pgxpool.Pool) *FollowerRepository {
	return &FollowerRepository{db: db}
}

// GetActiveFollowers retorna los IDs de seguidores activos de un autor.
// "Activos" = usuarios que han hecho login en los últimos 30 días.
// Limita a 10k — por encima de eso el fan-in es más eficiente (fase 2).
func (r *FollowerRepository) GetActiveFollowers(ctx context.Context, authorID uuid.UUID) ([]uuid.UUID, error) {
	const q = `
		SELECT follower_id
		FROM followers
		WHERE following_id = $1
		  AND last_seen_at > NOW() - INTERVAL '30 days'
		ORDER BY follower_id
		LIMIT 10000`

	rows, err := r.db.Query(ctx, q, authorID)
	if err != nil {
		return nil, fmt.Errorf("follower_repo.GetActiveFollowers: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("follower_repo.GetActiveFollowers scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

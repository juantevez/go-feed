package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/juantevez/my-ig/feed-service/internal/domain/feed"
	"github.com/juantevez/my-ig/shared/outbox"
)

// FeedRepository implementa feed.Repository sobre Postgres.
// Usa la tx del context si existe (outbox.Conn), así participa de TxManager.WithinTx.
type FeedRepository struct {
	db *pgxpool.Pool
}

func NewFeedRepository(db *pgxpool.Pool) *FeedRepository {
	return &FeedRepository{db: db}
}

// BulkInsert inserta múltiples entradas en un único round-trip usando pgx.Batch.
// No usamos CopyFrom porque COPY no soporta ON CONFLICT: ante un redelivery
// del evento fallaría por UNIQUE (user_id, post_id).
// ON CONFLICT DO NOTHING garantiza idempotencia ante redelivery de eventos.
func (r *FeedRepository) BulkInsert(ctx context.Context, entries []*feed.FeedEntry) error {
	const q = `
		INSERT INTO feed_entries (id, user_id, post_id, author_id, score, inserted_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id, post_id) DO NOTHING`

	batch := &pgx.Batch{}
	for _, e := range entries {
		batch.Queue(q, e.ID, e.UserID, e.PostID, e.AuthorID, e.Score, e.InsertedAt)
	}

	if err := r.db.SendBatch(ctx, batch).Close(); err != nil {
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
		rows, err = outbox.Conn(ctx, r.db).Query(ctx, q, userID, limit)
	} else {
		// Páginas siguientes — cursor compuesto (score, post_id)
		const q = `
			SELECT id, user_id, post_id, author_id, score, inserted_at
			FROM feed_entries
			WHERE user_id = $1
			  AND (score, post_id::text) < ($2, $3)
			ORDER BY score DESC, post_id DESC
			LIMIT $4`
		rows, err = outbox.Conn(ctx, r.db).Query(ctx, q, userID, cursor.Score, cursor.PostID.String(), limit)
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
	if _, err := outbox.Conn(ctx, r.db).Exec(ctx, q, postID); err != nil {
		return fmt.Errorf("feed_repo.DeleteByPostID: %w", err)
	}
	return nil
}

// DeleteByAuthorFromUser elimina del feed de un usuario los posts de un autor.
// Llamado en unfollow.
func (r *FeedRepository) DeleteByAuthorFromUser(ctx context.Context, userID, authorID uuid.UUID) error {
	const q = `DELETE FROM feed_entries WHERE user_id = $1 AND author_id = $2`
	if _, err := outbox.Conn(ctx, r.db).Exec(ctx, q, userID, authorID); err != nil {
		return fmt.Errorf("feed_repo.DeleteByAuthorFromUser: %w", err)
	}
	return nil
}

// ── FollowerRepository ────────────────────────────────────────────────────────

// FollowerRepository implementa feed.FollowerRepository.
// Usa la tx del context si existe (outbox.Conn), así participa de TxManager.WithinTx.
// El feed-service lee el grafo social de su propia DB (replicado desde social-service).
// En fase 1 la tabla followers es poblada directamente desde auth-service via evento.
type FollowerRepository struct {
	db *pgxpool.Pool
}

func NewFollowerRepository(db *pgxpool.Pool) *FollowerRepository {
	return &FollowerRepository{db: db}
}

// SaveFollower aplica un evento users.followed.v1 ocurrido en `at`.
// Last-write-wins: solo se aplica si es más nuevo que el último evento de esa
// relación (state_at). Retorna false si el evento era viejo y se descartó.
func (r *FollowerRepository) SaveFollower(ctx context.Context, followerID, followingID uuid.UUID, at time.Time) (bool, error) {
	const q = `
		INSERT INTO followers (follower_id, following_id, active, state_at, last_seen_at, created_at)
		VALUES ($1, $2, TRUE, $3, NOW(), NOW())
		ON CONFLICT (follower_id, following_id) DO UPDATE
		SET active = TRUE, state_at = EXCLUDED.state_at, last_seen_at = NOW()
		WHERE followers.state_at < EXCLUDED.state_at`
	tag, err := outbox.Conn(ctx, r.db).Exec(ctx, q, followerID, followingID, at)
	if err != nil {
		return false, fmt.Errorf("follower_repo.SaveFollower: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// MarkUnfollowed aplica un evento users.unfollowed.v1 ocurrido en `at`.
// No borra la fila: la deja inactiva (tombstone) para que un follow más viejo
// que llegue después no la reviva. Si el unfollow llega antes que su follow,
// inserta directamente el tombstone. Retorna false si el evento era viejo.
func (r *FollowerRepository) MarkUnfollowed(ctx context.Context, followerID, followingID uuid.UUID, at time.Time) (bool, error) {
	const q = `
		INSERT INTO followers (follower_id, following_id, active, state_at, last_seen_at, created_at)
		VALUES ($1, $2, FALSE, $3, NOW(), NOW())
		ON CONFLICT (follower_id, following_id) DO UPDATE
		SET active = FALSE, state_at = EXCLUDED.state_at
		WHERE followers.state_at < EXCLUDED.state_at`
	tag, err := outbox.Conn(ctx, r.db).Exec(ctx, q, followerID, followingID, at)
	if err != nil {
		return false, fmt.Errorf("follower_repo.MarkUnfollowed: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// GetActiveFollowers retorna los IDs de seguidores activos de un autor.
// "Activos" = usuarios que han hecho login en los últimos 30 días.
// Limita a 10k — por encima de eso el fan-in es más eficiente (fase 2).
func (r *FollowerRepository) GetActiveFollowers(ctx context.Context, authorID uuid.UUID) ([]uuid.UUID, error) {
	const q = `
		SELECT follower_id
		FROM followers
		WHERE following_id = $1
		  AND active
		  AND last_seen_at > NOW() - INTERVAL '30 days'
		ORDER BY follower_id
		LIMIT 10000`

	rows, err := outbox.Conn(ctx, r.db).Query(ctx, q, authorID)
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

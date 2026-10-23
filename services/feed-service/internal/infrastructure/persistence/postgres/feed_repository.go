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

	if err := outbox.Conn(ctx, r.db).SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("feed_repo.BulkInsert: %w", err)
	}
	return nil
}

// InsertForFollowers reparte el post a todos los seguidores con un INSERT ... SELECT:
// las filas no pasan por Go, así que no hace falta tope ni batches, y un autor con
// muchos seguidores no tiene una lista truncada. Usa idx_followers_following.
func (r *FeedRepository) InsertForFollowers(ctx context.Context, postID, authorID uuid.UUID, publishedAt time.Time) (int64, error) {
	const q = `
		INSERT INTO feed_entries (user_id, post_id, author_id, score)
		SELECT follower_id, $1, $2, $3
		FROM followers
		WHERE following_id = $2
		  AND active
		  AND follower_id <> $2
		ON CONFLICT (user_id, post_id) DO NOTHING`
	tag, err := outbox.Conn(ctx, r.db).Exec(ctx, q, postID, authorID, feed.Score(publishedAt))
	if err != nil {
		return 0, fmt.Errorf("feed_repo.InsertForFollowers: %w", err)
	}
	return tag.RowsAffected(), nil
}

// GetPage retorna entradas del feed paginadas con cursor-based pagination.
// El cursor usa (score DESC, post_id) para manejar empates de timestamp.
// La comparación de fila va sobre el uuid (no sobre su texto) para que entera sea
// Index Cond de idx_fe_user_score, en el mismo orden que el ORDER BY.
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
			  AND (score, post_id) < ($2, $3)
			ORDER BY score DESC, post_id DESC
			LIMIT $4`
		rows, err = outbox.Conn(ctx, r.db).Query(ctx, q, userID, cursor.Score, cursor.PostID, limit)
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

// LockPost toma un advisory lock de transacción sobre el post: create y delete
// del mismo post corren de a uno, así el chequeo del tombstone no tiene carrera.
// Fuera de una tx el lock se liberaría en el acto, por eso lo exige.
func (r *FeedRepository) LockPost(ctx context.Context, postID uuid.UUID) error {
	const q = `SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`
	if _, err := outbox.Conn(ctx, r.db).Exec(ctx, q, postID); err != nil {
		return fmt.Errorf("feed_repo.LockPost: %w", err)
	}
	return nil
}

// MarkPostDeleted registra el tombstone del post. ON CONFLICT lo hace idempotente
// ante reentregas de posts.deleted.
func (r *FeedRepository) MarkPostDeleted(ctx context.Context, postID uuid.UUID, deletedAt time.Time) error {
	const q = `
		INSERT INTO deleted_posts (post_id, deleted_at)
		VALUES ($1, $2)
		ON CONFLICT (post_id) DO NOTHING`
	if _, err := outbox.Conn(ctx, r.db).Exec(ctx, q, postID, deletedAt); err != nil {
		return fmt.Errorf("feed_repo.MarkPostDeleted: %w", err)
	}
	return nil
}

// IsPostDeleted indica si el post tiene tombstone en deleted_posts.
func (r *FeedRepository) IsPostDeleted(ctx context.Context, postID uuid.UUID) (bool, error) {
	const q = `SELECT EXISTS (SELECT 1 FROM deleted_posts WHERE post_id = $1)`
	var deleted bool
	if err := outbox.Conn(ctx, r.db).QueryRow(ctx, q, postID).Scan(&deleted); err != nil {
		return false, fmt.Errorf("feed_repo.IsPostDeleted: %w", err)
	}
	return deleted, nil
}

// SaveAuthorPost registra el post en author_posts. ON CONFLICT lo hace idempotente
// ante reentregas de posts.created.
func (r *FeedRepository) SaveAuthorPost(ctx context.Context, postID, authorID uuid.UUID, publishedAt time.Time) error {
	const q = `
		INSERT INTO author_posts (post_id, author_id, score)
		VALUES ($1, $2, $3)
		ON CONFLICT (post_id) DO NOTHING`
	if _, err := outbox.Conn(ctx, r.db).Exec(ctx, q, postID, authorID, feed.Score(publishedAt)); err != nil {
		return fmt.Errorf("feed_repo.SaveAuthorPost: %w", err)
	}
	return nil
}

// DeleteAuthorPost saca el post del índice author_posts. Llamado en posts.deleted.
func (r *FeedRepository) DeleteAuthorPost(ctx context.Context, postID uuid.UUID) error {
	const q = `DELETE FROM author_posts WHERE post_id = $1`
	if _, err := outbox.Conn(ctx, r.db).Exec(ctx, q, postID); err != nil {
		return fmt.Errorf("feed_repo.DeleteAuthorPost: %w", err)
	}
	return nil
}

// BackfillFromAuthor copia al feed de userID los `limit` posts más recientes
// de authorID. Llamado al empezar a seguir a alguien. Retorna cuántos insertó.
func (r *FeedRepository) BackfillFromAuthor(ctx context.Context, userID, authorID uuid.UUID, limit int) (int64, error) {
	const q = `
		INSERT INTO feed_entries (user_id, post_id, author_id, score)
		SELECT $1, post_id, author_id, score
		FROM author_posts
		WHERE author_id = $2
		ORDER BY score DESC
		LIMIT $3
		ON CONFLICT (user_id, post_id) DO NOTHING`
	tag, err := outbox.Conn(ctx, r.db).Exec(ctx, q, userID, authorID, limit)
	if err != nil {
		return 0, fmt.Errorf("feed_repo.BackfillFromAuthor: %w", err)
	}
	return tag.RowsAffected(), nil
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

// FollowerRepository mantiene la copia local del grafo social (tabla followers),
// que el fan-out lee con FeedRepository.InsertForFollowers. Usa la tx del context si existe (outbox.Conn), así participa de TxManager.WithinTx.
// El feed-service lee el grafo social de su propia DB, replicado desde social-service
// vía users.followed.v1 / users.unfollowed.v1.
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

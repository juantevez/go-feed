package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/juantevez/my-ig/content-service/internal/domain/post"
)

// PostRepository implementa post.Repository sobre Postgres.
type PostRepository struct {
	db *pgxpool.Pool
}

func NewPostRepository(db *pgxpool.Pool) *PostRepository {
	return &PostRepository{db: db}
}

func (r *PostRepository) Save(ctx context.Context, p *post.Post) error {
	const q = `
		INSERT INTO posts (id, author_id, caption, status, visibility, media_urls, tags, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err := r.db.Exec(ctx, q,
		p.ID, p.AuthorID, p.Caption,
		string(p.Status), string(p.Visibility),
		p.MediaURLs, p.Tags,
		p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("post_repo.Save: %w", err)
	}
	return nil
}

func (r *PostRepository) FindByID(ctx context.Context, id uuid.UUID) (*post.Post, error) {
	const q = `
		SELECT id, author_id, caption, status, visibility, media_urls, tags,
		       created_at, updated_at, deleted_at
		FROM posts WHERE id = $1`

	p, err := scanPost(r.db.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, post.ErrPostNotFound
		}
		return nil, fmt.Errorf("post_repo.FindByID: %w", err)
	}
	return p, nil
}

func (r *PostRepository) Update(ctx context.Context, p *post.Post) error {
	const q = `
		UPDATE posts
		SET caption = $1, status = $2, visibility = $3,
		    media_urls = $4, tags = $5, updated_at = $6, deleted_at = $7
		WHERE id = $8`

	tag, err := r.db.Exec(ctx, q,
		p.Caption, string(p.Status), string(p.Visibility),
		p.MediaURLs, p.Tags, p.UpdatedAt, p.DeletedAt,
		p.ID,
	)
	if err != nil {
		return fmt.Errorf("post_repo.Update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return post.ErrPostNotFound
	}
	return nil
}

func (r *PostRepository) FindByAuthor(ctx context.Context, authorID uuid.UUID, limit, offset int) ([]*post.Post, error) {
	const q = `
		SELECT id, author_id, caption, status, visibility, media_urls, tags,
		       created_at, updated_at, deleted_at
		FROM posts
		WHERE author_id = $1 AND status != 'deleted'
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.db.Query(ctx, q, authorID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("post_repo.FindByAuthor: %w", err)
	}
	defer rows.Close()

	var posts []*post.Post
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, fmt.Errorf("post_repo.FindByAuthor scan: %w", err)
		}
		posts = append(posts, p)
	}
	return posts, rows.Err()
}

// scanPost mapea una row de Postgres al agregado Post.
// pgx maneja TEXT[] nativamente como []string.
func scanPost(row pgx.Row) (*post.Post, error) {
	var p post.Post
	var status, visibility string
	err := row.Scan(
		&p.ID, &p.AuthorID, &p.Caption,
		&status, &visibility,
		&p.MediaURLs, &p.Tags,
		&p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
	)
	if err != nil {
		return nil, err
	}
	p.Status = post.Status(status)
	p.Visibility = post.Visibility(visibility)
	return &p, nil
}

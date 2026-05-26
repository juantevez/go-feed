package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"feed-backend/services/feed/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresRepo(ctx context.Context, dsn string) (*PostgresRepo, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}

	// Optimizaciones para servicio de lectura frecuente
	config.MaxConns = 20
	config.MinConns = 5
	config.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("pgxpool new: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("pgxpool ping: %w", err)
	}
	return &PostgresRepo{pool: pool}, nil
}

// GetFollowers obtiene lista de follower_id para fan-out
func (r *PostgresRepo) GetFollowers(ctx context.Context, authorID string) ([]string, error) {
	query := `
		SELECT follower_id FROM follows
		WHERE following_id = $1
		ORDER BY created_at DESC
	`
	rows, err := r.pool.Query(ctx, query, authorID)
	if err != nil {
		return nil, fmt.Errorf("query followers: %w", err)
	}
	defer rows.Close()

	var followers []string
	for rows.Next() {
		var fid string
		if err := rows.Scan(&fid); err != nil {
			return nil, fmt.Errorf("scan follower: %w", err)
		}
		followers = append(followers, fid)
	}
	return followers, nil
}

// GetPostMeta recupera metadata de un post (filtra soft-delete y moderación)
func (r *PostgresRepo) GetPostMeta(ctx context.Context, postID string) (*domain.PostMeta, error) {
	query := `
		SELECT id, author_id, caption, media_urls, created_at, likes_count, comments_count
		FROM posts
		WHERE id = $1
		  AND deleted_at IS NULL
		  AND moderation_status = 'approved'
		  AND visibility IN ('public', 'followers_only')
	`
	var p domain.PostMeta
	var mediaJSON []byte
	var createdAt time.Time

	err := r.pool.QueryRow(ctx, query, postID).Scan(
		&p.ID, &p.AuthorID, &p.Caption, &mediaJSON, &createdAt, &p.Likes, &p.Comments,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("post not found or not visible")
		}
		return nil, fmt.Errorf("query post meta: %w", err)
	}

	p.CreatedAt = createdAt.UTC().Format(time.RFC3339)

	// Parsear JSONB media_urls de forma segura
	if len(mediaJSON) > 0 && string(mediaJSON) != "null" {
		if err := json.Unmarshal(mediaJSON, &p.MediaURLs); err != nil {
			p.MediaURLs = []string{}
		}
	}
	return &p, nil
}

// GetUsername obtiene username para renderizado del feed
func (r *PostgresRepo) GetUsername(ctx context.Context, userID string) (string, error) {
	query := `SELECT username FROM users WHERE id = $1 AND deleted_at IS NULL`
	var username string
	err := r.pool.QueryRow(ctx, query, userID).Scan(&username)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "deleted_user", nil
		}
		return "", fmt.Errorf("query username: %w", err)
	}
	return username, nil
}

// Close libera el pool de conexiones
func (r *PostgresRepo) Close() {
	r.pool.Close()
}

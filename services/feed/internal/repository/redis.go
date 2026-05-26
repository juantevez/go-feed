package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisRepo struct {
	client *redis.Client
}

func NewRedisRepo(addr, password string, db int) (*RedisRepo, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return &RedisRepo{client: rdb}, nil
}

// PushToFeed inserta post_id en el timeline ordenado del usuario
func (r *RedisRepo) PushToFeed(ctx context.Context, userID, postID string, score float64) error {
	key := fmt.Sprintf("feed:%s", userID)
	return r.client.ZAdd(ctx, key, redis.Z{Score: score, Member: postID}).Err()
}

// GetFeedPostIDs obtiene IDs con cursor. Retorna: ids, next_cursor_raw, error
func (r *RedisRepo) GetFeedPostIDs(ctx context.Context, userID, cursor string, limit int) ([]string, string, error) {
	key := fmt.Sprintf("feed:%s", userID)

	// Cursor formato: "timestamp_postID". Si está vacío, trae los más nuevos.
	max := "+inf"
	if cursor != "" {
		max = fmt.Sprintf("(%s", cursor) // Exclusivo para paginación estable
	}

	res, err := r.client.ZRevRangeByScoreWithScores(ctx, key, &redis.ZRangeBy{
		Max:    max,
		Min:    "-inf",
		Offset: 0,
		Count:  int64(limit + 1), // +1 para saber si hay más páginas
	}).Result()
	if err != nil {
		return nil, "", fmt.Errorf("zrevrangebyscore: %w", err)
	}

	var ids []string
	var nextCursor string
	for i, z := range res {
		if i == limit {
			// El elemento N+1 define el siguiente cursor
			nextCursor = fmt.Sprintf("%v_%s", int64(z.Score), z.Member)
			break
		}
		ids = append(ids, z.Member.(string))
	}

	return ids, nextCursor, nil
}

package domain

import "context"

type FeedEntry struct {
	PostID    string   `json:"post_id"`
	AuthorID  string   `json:"author_id"`
	Username  string   `json:"username"`
	Caption   string   `json:"caption,omitempty"`
	MediaURLs []string `json:"media_urls"`
	Likes     int64    `json:"likes"`
	Comments  int64    `json:"comments"`
	CreatedAt string   `json:"created_at"`
	Cursor    string   `json:"cursor"`
}

type PostMeta struct {
	ID        string
	AuthorID  string
	Caption   string
	MediaURLs []string
	CreatedAt string
	Likes     int64
	Comments  int64
}

type FeedStore interface {
	// Redis: fan-out & cache
	PushToFeed(ctx context.Context, userID, postID string, score float64) error
	GetFeedPostIDs(ctx context.Context, userID, cursor string, limit int) ([]string, string, error)

	// DB: metadatos & grafo
	GetPostMeta(ctx context.Context, postID string) (*PostMeta, error)
	GetUsername(ctx context.Context, userID string) (string, error)
	GetFollowers(ctx context.Context, authorID string) ([]string, error)
}

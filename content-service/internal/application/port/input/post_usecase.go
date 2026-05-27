package input

import (
	"context"
	"mime/multipart"

	"github.com/google/uuid"
	"github.com/juantevez/my-ig/content-service/internal/domain/post"
)

// ── CreatePost ────────────────────────────────────────────────────────────────

type CreatePostCommand struct {
	AuthorID   uuid.UUID
	Caption    string
	Visibility post.Visibility
	Files      []*multipart.FileHeader // archivos subidos por el cliente
	TraceID    string
}

type CreatePostResult struct {
	Post *post.Post
}

// ── GetPost ───────────────────────────────────────────────────────────────────

type GetPostQuery struct {
	PostID         uuid.UUID
	RequestingUser uuid.UUID // para validar visibilidad
}

type GetPostResult struct {
	Post *post.Post
}

// ── DeletePost ────────────────────────────────────────────────────────────────

type DeletePostCommand struct {
	PostID   uuid.UUID
	AuthorID uuid.UUID // quien hace el request — validado contra post.AuthorID
	Reason   string
	TraceID  string
}

// ── PostUseCase ───────────────────────────────────────────────────────────────

// PostUseCase es el puerto driving principal del content-service.
type PostUseCase interface {
	CreatePost(ctx context.Context, cmd CreatePostCommand) (*CreatePostResult, error)
	GetPost(ctx context.Context, q GetPostQuery) (*GetPostResult, error)
	DeletePost(ctx context.Context, cmd DeletePostCommand) error
}

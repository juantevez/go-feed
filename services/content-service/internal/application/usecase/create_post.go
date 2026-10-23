package usecase

import (
	"context"
	"fmt"
	"log/slog"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/juantevez/my-ig/content-service/internal/application/port/input"
	"github.com/juantevez/my-ig/content-service/internal/application/port/output"
	"github.com/juantevez/my-ig/content-service/internal/domain/media"
	"github.com/juantevez/my-ig/content-service/internal/domain/post"
	"github.com/juantevez/my-ig/shared/events"
)

// PostService implementa input.PostUseCase.
type PostService struct {
	tx        output.TxManager
	posts     post.Repository
	store     output.MediaStore
	publisher output.EventPublisher
}

func NewPostService(
	tx output.TxManager,
	posts post.Repository,
	store output.MediaStore,
	publisher output.EventPublisher,
) *PostService {
	return &PostService{tx: tx, posts: posts, store: store, publisher: publisher}
}

// CreatePost orquesta el flujo completo:
//  1. Crea el agregado Post en draft
//  2. Valida y sube cada archivo de media a S3
//  3. Publica el post (draft → published)
//  4. Persiste el post y el evento posts.created.v1 en la misma tx (outbox)
//
// S3 no participa de la tx: si algo falla después de subir archivos,
// se borran los ya subidos (compensación best-effort).
func (s *PostService) CreatePost(ctx context.Context, cmd input.CreatePostCommand) (_ *input.CreatePostResult, err error) {
	// 1. Crear agregado en draft
	p, err := post.New(cmd.AuthorID, cmd.Caption, cmd.Visibility)
	if err != nil {
		return nil, fmt.Errorf("create_post: build post: %w", err)
	}

	// 2. Subir archivos de media a S3
	var mediaURLs []string
	defer func() {
		if err != nil && len(mediaURLs) > 0 {
			s.deleteMediaAsync(mediaURLs)
		}
	}()

	for _, fh := range cmd.Files {
		f, err := fh.Open()
		if err != nil {
			return nil, fmt.Errorf("create_post: open file %s: %w", fh.Filename, err)
		}
		defer f.Close()

		mimeType := fh.Header.Get("Content-Type")
		m, err := media.New(p.ID, fh.Filename, mimeType, fh.Size)
		if err != nil {
			return nil, fmt.Errorf("create_post: validate media %s: %w", fh.Filename, err)
		}

		url, err := s.store.Upload(ctx, m.S3Key, f, m.MIMEType, m.SizeBytes)
		if err != nil {
			return nil, fmt.Errorf("create_post: upload %s: %w", fh.Filename, err)
		}
		mediaURLs = append(mediaURLs, url)
	}

	// 3. Publicar (draft → published)
	if err := p.Publish(mediaURLs); err != nil {
		return nil, fmt.Errorf("create_post: publish: %w", err)
	}

	// 4. Persistir post + evento atómicamente
	traceID := chimiddleware.GetReqID(ctx)
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.posts.Save(ctx, p); err != nil {
			return fmt.Errorf("create_post: save: %w", err)
		}
		if err := s.publisher.Publish(ctx, events.TopicPostCreated, post.NewCreatedEnvelope(p, traceID)); err != nil {
			return fmt.Errorf("create_post: enqueue event: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &input.CreatePostResult{Post: p}, nil
}

// deleteMediaAsync borra archivos de S3 en background, sin bloquear la respuesta.
// Los errores se loguean: un archivo huérfano no debe fallar el request.
func (s *PostService) deleteMediaAsync(urls []string) {
	go func() {
		for _, url := range urls {
			if err := s.store.Delete(context.Background(), url); err != nil {
				slog.Error("media delete failed", "url", url, "err", err)
			}
		}
	}()
}

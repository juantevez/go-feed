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
	posts     post.Repository
	store     output.MediaStore
	publisher output.EventPublisher
}

func NewPostService(
	posts post.Repository,
	store output.MediaStore,
	publisher output.EventPublisher,
) *PostService {
	return &PostService{posts: posts, store: store, publisher: publisher}
}

// CreatePost orquesta el flujo completo:
//  1. Crea el agregado Post en draft
//  2. Valida y sube cada archivo de media a S3
//  3. Publica el post (draft → published)
//  4. Persiste en DB
//  5. Publica posts.created.v1 al bus (fire-and-forget)
func (s *PostService) CreatePost(ctx context.Context, cmd input.CreatePostCommand) (*input.CreatePostResult, error) {
	// 1. Crear agregado en draft
	p, err := post.New(cmd.AuthorID, cmd.Caption, cmd.Visibility)
	if err != nil {
		return nil, fmt.Errorf("create_post: build post: %w", err)
	}

	// 2. Subir archivos de media a S3
	var mediaURLs []string
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

	// 4. Persistir
	if err := s.posts.Save(ctx, p); err != nil {
		return nil, fmt.Errorf("create_post: save: %w", err)
	}

	// 5. Evento al bus — fire-and-forget
	traceID := chimiddleware.GetReqID(ctx)
	s.publishAsync(events.TopicPostCreated, post.NewCreatedEnvelope(p, traceID))

	return &input.CreatePostResult{Post: p}, nil
}

func (s *PostService) publishAsync(topic string, envelope events.Envelope) {
	go func() {
		if err := s.publisher.Publish(context.Background(), topic, envelope); err != nil {
			slog.Error("event publish failed",
				"topic", topic,
				"event_id", envelope.EventID,
				"err", err,
			)
		}
	}()
}

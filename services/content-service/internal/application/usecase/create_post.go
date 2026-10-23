package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"

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
//  2. Valida todos los archivos de media (tipo detectado por contenido, tamaño)
//  3. Sube cada archivo a S3
//  4. Publica el post (draft → published)
//  5. Persiste el post y el evento posts.created.v1 en la misma tx (outbox)
//
// Se valida todo antes de subir nada: un archivo inválido no deja objetos en S3.
// S3 no participa de la tx: si algo falla después de subir archivos,
// se borran los ya subidos (compensación best-effort).
func (s *PostService) CreatePost(ctx context.Context, cmd input.CreatePostCommand) (_ *input.CreatePostResult, err error) {
	// 1. Crear agregado en draft
	p, err := post.New(cmd.AuthorID, cmd.Caption, cmd.Visibility)
	if err != nil {
		return nil, fmt.Errorf("create_post: build post: %w", err)
	}
	if len(cmd.Files) > post.MaxMediaPerPost {
		return nil, fmt.Errorf("create_post: %w", post.ErrTooManyMedia)
	}

	// 2. Validar media
	medias := make([]*media.Media, len(cmd.Files))
	for i, fh := range cmd.Files {
		mimeType, err := detectMIMEType(fh)
		if err != nil {
			return nil, fmt.Errorf("create_post: read file %s: %w", fh.Filename, err)
		}
		medias[i], err = media.New(p.ID, !p.IsPublic(), fh.Filename, mimeType, fh.Size)
		if err != nil {
			return nil, fmt.Errorf("create_post: validate media: %w", err)
		}
	}

	// 3. Subir archivos de media a S3
	var mediaURLs []string
	defer func() {
		if err != nil && len(mediaURLs) > 0 {
			s.deleteMediaAsync(mediaURLs)
		}
	}()

	for i, fh := range cmd.Files {
		url, err := s.upload(ctx, fh, medias[i])
		if err != nil {
			return nil, fmt.Errorf("create_post: upload %s: %w", fh.Filename, err)
		}
		mediaURLs = append(mediaURLs, url)
	}

	// 4. Publicar (draft → published)
	if err := p.Publish(mediaURLs); err != nil {
		return nil, fmt.Errorf("create_post: publish: %w", err)
	}

	// 5. Persistir post + evento atómicamente
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

	if err := s.presignPrivateMedia(ctx, p); err != nil {
		return nil, fmt.Errorf("create_post: %w", err)
	}
	return &input.CreatePostResult{Post: p}, nil
}

// sniffLen es lo que mira http.DetectContentType: no usa más de 512 bytes.
const sniffLen = 512

// detectMIMEType determina el tipo del archivo por sus primeros bytes (magic numbers).
// El Content-Type que declara el cliente no se usa: se puede falsificar.
func detectMIMEType(fh *multipart.FileHeader) (string, error) {
	f, err := fh.Open()
	if err != nil {
		return "", err
	}
	defer f.Close()

	head := make([]byte, sniffLen)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", err
	}
	return http.DetectContentType(head[:n]), nil
}

// upload sube un archivo ya validado y retorna su URL pública.
func (s *PostService) upload(ctx context.Context, fh *multipart.FileHeader, m *media.Media) (string, error) {
	f, err := fh.Open()
	if err != nil {
		return "", err
	}
	defer f.Close()
	return s.store.Upload(ctx, m.S3Key, f, m.MIMEType, m.SizeBytes)
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

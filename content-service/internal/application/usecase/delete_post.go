package usecase

import (
	"context"
	"fmt"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/juantevez/my-ig/content-service/internal/application/port/input"
	"github.com/juantevez/my-ig/content-service/internal/domain/post"
	"github.com/juantevez/my-ig/shared/events"
)

// DeletePost hace soft-delete del post, elimina el media de S3
// y publica posts.deleted.v1 al bus.
func (s *PostService) DeletePost(ctx context.Context, cmd input.DeletePostCommand) error {
	p, err := s.posts.FindByID(ctx, cmd.PostID)
	if err != nil {
		return fmt.Errorf("delete_post: find: %w", err)
	}

	// La lógica de autorización vive en el agregado.
	if err := p.Delete(cmd.AuthorID); err != nil {
		return fmt.Errorf("delete_post: %w", err)
	}

	// Persistir estado deleted.
	if err := s.posts.Update(ctx, p); err != nil {
		return fmt.Errorf("delete_post: update: %w", err)
	}

	// Eliminar media de S3 en background — no bloquea la respuesta.
	go func() {
		for _, url := range p.MediaURLs {
			if err := s.store.Delete(context.Background(), url); err != nil {
				// Log y continuar — el archivo puede limpiarse luego con un job.
				_ = err
			}
		}
	}()

	// Evento al bus — fire-and-forget.
	traceID := chimiddleware.GetReqID(ctx)
	s.publishAsync(events.TopicPostDeleted, post.NewDeletedEnvelope(p, cmd.Reason, traceID))

	return nil
}

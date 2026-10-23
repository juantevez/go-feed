package usecase

import (
	"context"
	"fmt"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/juantevez/my-ig/content-service/internal/application/port/input"
	"github.com/juantevez/my-ig/content-service/internal/domain/post"
	"github.com/juantevez/my-ig/shared/events"
)

// DeletePost hace soft-delete del post y encola posts.deleted.v1 en la misma tx
// (outbox). Después del commit elimina el media de S3.
func (s *PostService) DeletePost(ctx context.Context, cmd input.DeletePostCommand) error {
	traceID := chimiddleware.GetReqID(ctx)

	var p *post.Post
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		p, err = s.posts.FindByID(ctx, cmd.PostID)
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

		envelope := post.NewDeletedEnvelope(p, cmd.Reason, traceID)
		if err := s.publisher.Publish(ctx, events.TopicPostDeleted, envelope); err != nil {
			return fmt.Errorf("delete_post: enqueue event: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Solo después del commit: si la tx falla, el post sigue vivo y necesita sus archivos.
	s.deleteMediaAsync(p.MediaURLs)

	return nil
}

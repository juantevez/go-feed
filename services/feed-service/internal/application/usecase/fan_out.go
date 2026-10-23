package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/juantevez/my-ig/feed-service/internal/application/port/input"
	"github.com/juantevez/my-ig/feed-service/internal/application/port/output"
	"github.com/juantevez/my-ig/feed-service/internal/domain/feed"
)

// visibilityPublic es el único valor de visibility que llega al feed de los seguidores.
// followers_only también queda afuera: content-service todavía no consulta el grafo
// social y les responde 404 a los seguidores.
const visibilityPublic = "public"

// FanOutService implementa input.FanOutUseCase.
// Consume posts.created.v1 y propaga el post al feed del autor y de cada seguidor activo.
type FanOutService struct {
	tx    output.TxManager
	feeds feed.Repository
}

func NewFanOutService(tx output.TxManager, feeds feed.Repository) *FanOutService {
	return &FanOutService{tx: tx, feeds: feeds}
}

// FanOut propaga el post al feed del autor y, si es público, a los de todos sus
// seguidores activos. Un post no público solo llega al feed del autor.
//
// Estrategia: fan-out sincrónico en la DB (INSERT ... SELECT sobre followers),
// sin tope de seguidores. Fan-in lazy para autores enormes queda para fase 2.
//
// Idempotencia: las inserciones usan ON CONFLICT DO NOTHING — si el worker
// reprocesa el mismo evento, no genera duplicados.
//
// Corre en una tx con el post lockeado: si el posts.deleted del mismo post ya se
// procesó (tombstone), no hace nada; si llega mientras tanto, espera al commit y
// borra lo que se insertó. Así el orden de llegada de los dos eventos no importa.
func (s *FanOutService) FanOut(ctx context.Context, cmd input.FanOutCommand) error {
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.feeds.LockPost(ctx, cmd.PostID); err != nil {
			return fmt.Errorf("fan_out: lock post: %w", err)
		}
		deleted, err := s.feeds.IsPostDeleted(ctx, cmd.PostID)
		if err != nil {
			return fmt.Errorf("fan_out: check deleted: %w", err)
		}
		if deleted {
			slog.Info("fan_out: post already deleted, skipping", "post_id", cmd.PostID, "author_id", cmd.AuthorID)
			return nil
		}
		return s.fanOut(ctx, cmd)
	})
}

func (s *FanOutService) fanOut(ctx context.Context, cmd input.FanOutCommand) error {
	// El autor siempre ve sus propios posts en su timeline, de cualquier visibilidad.
	own, err := feed.New(cmd.AuthorID, cmd.PostID, cmd.AuthorID, cmd.PublishedAt)
	if err != nil {
		return fmt.Errorf("fan_out: build author entry: %w", err)
	}
	if err := s.feeds.BulkInsert(ctx, []*feed.FeedEntry{own}); err != nil {
		return fmt.Errorf("fan_out: insert author entry: %w", err)
	}

	if cmd.Visibility != visibilityPublic {
		slog.Info("fan_out: non-public post, author feed only",
			"post_id", cmd.PostID, "author_id", cmd.AuthorID, "visibility", cmd.Visibility)
		return nil
	}

	// Índice por autor: lo usa el backfill cuando alguien empieza a seguirlo,
	// así que solo lleva posts públicos.
	if err := s.feeds.SaveAuthorPost(ctx, cmd.PostID, cmd.AuthorID, cmd.PublishedAt); err != nil {
		return fmt.Errorf("fan_out: save author post: %w", err)
	}

	n, err := s.feeds.InsertForFollowers(ctx, cmd.PostID, cmd.AuthorID, cmd.PublishedAt)
	if err != nil {
		return fmt.Errorf("fan_out: insert for followers: %w", err)
	}

	slog.Info("fan_out: done", "post_id", cmd.PostID, "author_id", cmd.AuthorID, "follower_entries", n)
	return nil
}

package natsconsumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"

	"github.com/juantevez/my-ig/feed-service/internal/application/port/input"
	"github.com/juantevez/my-ig/shared/dlq"
	"github.com/juantevez/my-ig/shared/events"
)

// Consumer suscribe al bus NATS JetStream y despacha eventos
// a los use cases correspondientes.
type Consumer struct {
	js           nats.JetStreamContext
	tx           txRunner
	fanOut       input.FanOutUseCase
	feedRepo     feedWriter
	followerRepo followerWriter
}

// txRunner ejecuta fn dentro de una transacción (outbox.TxManager).
type txRunner interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// backfillLimit es cuántos posts recientes del autor se cargan al seguirlo.
const backfillLimit = 20

// feedWriter es la interfaz mínima que el consumer necesita sobre el feed.
type feedWriter interface {
	LockPost(ctx context.Context, postID uuid.UUID) error
	MarkPostDeleted(ctx context.Context, postID uuid.UUID, deletedAt time.Time) error
	DeleteByPostID(ctx context.Context, postID uuid.UUID) error
	DeleteAuthorPost(ctx context.Context, postID uuid.UUID) error
	DeleteByAuthorFromUser(ctx context.Context, userID, authorID uuid.UUID) error
	BackfillFromAuthor(ctx context.Context, userID, authorID uuid.UUID, limit int) (int64, error)
}

// followerWriter sincroniza el grafo social en la DB local del feed-service.
// Ambos métodos son last-write-wins por timestamp del evento: followed y
// unfollowed llegan por subscriptions paralelas y pueden procesarse desordenados.
// Retornan false si el evento era más viejo que el estado actual (se descarta).
type followerWriter interface {
	SaveFollower(ctx context.Context, followerID, followingID uuid.UUID, at time.Time) (bool, error)
	MarkUnfollowed(ctx context.Context, followerID, followingID uuid.UUID, at time.Time) (bool, error)
}

func NewConsumer(js nats.JetStreamContext, tx txRunner, fanOut input.FanOutUseCase, feedRepo feedWriter, followerRepo followerWriter) *Consumer {
	return &Consumer{js: js, tx: tx, fanOut: fanOut, feedRepo: feedRepo, followerRepo: followerRepo}
}

// Subscribe registra todos los consumers JetStream del feed-service.
// Debe llamarse una sola vez al arrancar el servicio.
//
// Los handlers retornan error en vez de hacer Ack/Nak: dlq.Subscribe aplica
// reintentos con backoff y, si se agotan (o el error es dlq.Permanent), manda
// el mensaje al stream DLQ.
func (c *Consumer) Subscribe() error {
	if err := dlq.EnsureStream(c.js); err != nil {
		return err
	}

	subs := []struct {
		topic   string
		durable string
		handler dlq.Handler
	}{
		{events.TopicPostCreated, "feed-service-post-created", c.handlePostCreated},
		{events.TopicPostDeleted, "feed-service-post-deleted", c.handlePostDeleted},
		{events.TopicUserFollowed, "feed-service-user-followed", c.handleUserFollowed},
		{events.TopicUserUnfollowed, "feed-service-user-unfollowed", c.handleUserUnfollowed},
	}
	for _, sub := range subs {
		if err := dlq.Subscribe(c.js, sub.topic, sub.durable, sub.handler); err != nil {
			return fmt.Errorf("consumer: %w", err)
		}
		slog.Info("consumer: subscribed", "topic", sub.topic)
	}
	return nil
}

// ── posts.created.v1 ──────────────────────────────────────────────────────────

func (c *Consumer) handlePostCreated(ctx context.Context, msg *nats.Msg) error {
	var envelope events.Envelope
	if err := json.Unmarshal(msg.Data, &envelope); err != nil {
		// Payload malformado: reintentar no lo va a arreglar.
		return dlq.Permanent(fmt.Errorf("unmarshal envelope: %w", err))
	}

	// El payload viene como map[string]any por el any del Envelope.
	// Lo re-serializamos al tipo concreto.
	cmd, err := extractPostCreatedCommand(envelope)
	if err != nil {
		return dlq.Permanent(fmt.Errorf("extract post created payload (event_id=%s): %w", envelope.EventID, err))
	}

	slog.InfoContext(ctx, "consumer: processing post created",
		"event_id", envelope.EventID,
		"post_id", cmd.PostID,
		"author_id", cmd.AuthorID,
	)

	if err := c.fanOut.FanOut(ctx, cmd); err != nil {
		return fmt.Errorf("fan out (post_id=%s): %w", cmd.PostID, err)
	}
	return nil
}

// ── posts.deleted.v1 ──────────────────────────────────────────────────────────

func (c *Consumer) handlePostDeleted(ctx context.Context, msg *nats.Msg) error {
	var envelope events.Envelope
	if err := json.Unmarshal(msg.Data, &envelope); err != nil {
		return dlq.Permanent(fmt.Errorf("unmarshal envelope: %w", err))
	}

	var p events.PostDeletedPayload
	if err := decodePayload(envelope, &p); err != nil {
		return dlq.Permanent(fmt.Errorf("extract post deleted payload (event_id=%s): %w", envelope.EventID, err))
	}
	postID := p.PostID

	slog.InfoContext(ctx, "consumer: invalidating feed for deleted post", "post_id", postID)

	// Mismo lock que el fan-out: si el posts.created de este post se está
	// procesando, esperamos su commit y borramos lo que insertó. El tombstone
	// frena a un posts.created que llegue después.
	return c.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := c.feedRepo.LockPost(ctx, postID); err != nil {
			return fmt.Errorf("lock post (post_id=%s): %w", postID, err)
		}
		if err := c.feedRepo.MarkPostDeleted(ctx, postID, p.DeletedAt); err != nil {
			return fmt.Errorf("mark post deleted (post_id=%s): %w", postID, err)
		}
		if err := c.feedRepo.DeleteByPostID(ctx, postID); err != nil {
			return fmt.Errorf("delete feed entries (post_id=%s): %w", postID, err)
		}
		// Sin esto, un follow posterior lo volvería a cargar vía backfill.
		if err := c.feedRepo.DeleteAuthorPost(ctx, postID); err != nil {
			return fmt.Errorf("delete author post (post_id=%s): %w", postID, err)
		}
		return nil
	})
}

// ── users.followed.v1 ─────────────────────────────────────────────────────────

func (c *Consumer) handleUserFollowed(ctx context.Context, msg *nats.Msg) error {
	var envelope events.Envelope
	if err := json.Unmarshal(msg.Data, &envelope); err != nil {
		return dlq.Permanent(fmt.Errorf("unmarshal envelope: %w", err))
	}

	var p events.UserFollowedPayload
	if err := decodePayload(envelope, &p); err != nil {
		return dlq.Permanent(fmt.Errorf("extract user followed payload (event_id=%s): %w", envelope.EventID, err))
	}

	slog.InfoContext(ctx, "consumer: saving follower", "follower_id", p.FollowerID, "following_id", p.FollowingID)

	// Follow + backfill en la misma tx, por la misma razón que en unfollow:
	// si el backfill fallara, el reintento vería el follow como stale y lo saltearía.
	return c.tx.WithinTx(ctx, func(ctx context.Context) error {
		applied, err := c.followerRepo.SaveFollower(ctx, p.FollowerID, p.FollowingID, p.FollowedAt)
		if err != nil {
			return fmt.Errorf("save follower: %w", err)
		}
		if !applied {
			// Hay un unfollow más nuevo: no cargar sus posts.
			slog.InfoContext(ctx, "consumer: stale follow event ignored", "event_id", envelope.EventID,
				"follower_id", p.FollowerID, "following_id", p.FollowingID)
			return nil
		}

		// Cargar los posts recientes del autor en el feed del nuevo seguidor.
		n, err := c.feedRepo.BackfillFromAuthor(ctx, p.FollowerID, p.FollowingID, backfillLimit)
		if err != nil {
			return fmt.Errorf("backfill feed: %w", err)
		}
		slog.InfoContext(ctx, "consumer: feed backfilled", "follower_id", p.FollowerID,
			"following_id", p.FollowingID, "entries", n)
		return nil
	})
}

// ── users.unfollowed.v1 ───────────────────────────────────────────────────────

func (c *Consumer) handleUserUnfollowed(ctx context.Context, msg *nats.Msg) error {
	var envelope events.Envelope
	if err := json.Unmarshal(msg.Data, &envelope); err != nil {
		return dlq.Permanent(fmt.Errorf("unmarshal envelope: %w", err))
	}

	var p events.UserUnfollowedPayload
	if err := decodePayload(envelope, &p); err != nil {
		return dlq.Permanent(fmt.Errorf("extract user unfollowed payload (event_id=%s): %w", envelope.EventID, err))
	}

	slog.InfoContext(ctx, "consumer: removing follower", "follower_id", p.FollowerID, "following_id", p.FollowingID)

	// Tombstone + limpieza del feed en la misma tx: si el borrado fallara después
	// de aplicar el tombstone, el reintento vería el evento como stale y nunca
	// limpiaría el feed.
	return c.tx.WithinTx(ctx, func(ctx context.Context) error {
		applied, err := c.followerRepo.MarkUnfollowed(ctx, p.FollowerID, p.FollowingID, p.UnfollowedAt)
		if err != nil {
			return fmt.Errorf("mark unfollowed: %w", err)
		}
		if !applied {
			// Hay un follow más nuevo: sus posts deben quedar en el feed.
			slog.InfoContext(ctx, "consumer: stale unfollow event ignored", "event_id", envelope.EventID,
				"follower_id", p.FollowerID, "following_id", p.FollowingID)
			return nil
		}

		// Sacar del feed del follower los posts del autor que dejó de seguir.
		if err := c.feedRepo.DeleteByAuthorFromUser(ctx, p.FollowerID, p.FollowingID); err != nil {
			return fmt.Errorf("delete feed entries of unfollowed author: %w", err)
		}
		return nil
	})
}

// ── helpers de extracción de payload ─────────────────────────────────────────

// decodePayload re-serializa el payload any del Envelope al tipo concreto.
func decodePayload(env events.Envelope, dst any) error {
	raw, err := json.Marshal(env.Payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}

// extractPostCreatedCommand convierte el payload de posts.created.v1 al comando del use case.
func extractPostCreatedCommand(env events.Envelope) (input.FanOutCommand, error) {
	var p events.PostCreatedPayload
	if err := decodePayload(env, &p); err != nil {
		return input.FanOutCommand{}, err
	}
	return input.FanOutCommand{
		PostID:      p.PostID,
		AuthorID:    p.AuthorID,
		PublishedAt: p.CreatedAt,
		Visibility:  p.Visibility,
	}, nil
}

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/juantevez/my-ig/notification-service/internal/domain/notification"
	"github.com/juantevez/my-ig/shared/outbox"
)

// NotificationRepository implementa notification.Repository sobre Postgres.
// Usa la tx del context si existe (outbox.Conn), así participa de TxManager.WithinTx.
type NotificationRepository struct {
	db *pgxpool.Pool
}

func NewNotificationRepository(db *pgxpool.Pool) *NotificationRepository {
	return &NotificationRepository{db: db}
}

func (r *NotificationRepository) Save(ctx context.Context, n *notification.Notification) (bool, error) {
	payload, err := json.Marshal(n.Payload)
	if err != nil {
		return false, fmt.Errorf("notification_repo.Save: marshal payload: %w", err)
	}

	// ON CONFLICT sobre el índice parcial uq_notif_event_target: una reentrega
	// del mismo evento no duplica la notificación.
	const q = `
		INSERT INTO notifications (id, target_user_id, source_user_id, type, payload, read, created_at, event_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (event_id, target_user_id) WHERE event_id IS NOT NULL DO NOTHING`

	var eventID *uuid.UUID
	if n.EventID != uuid.Nil {
		eventID = &n.EventID
	}

	tag, err := outbox.Conn(ctx, r.db).Exec(ctx, q,
		n.ID, n.TargetUserID, n.SourceUserID,
		string(n.Type), payload, n.Read, n.CreatedAt, eventID,
	)
	if err != nil {
		return false, fmt.Errorf("notification_repo.Save: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *NotificationRepository) GetByUser(ctx context.Context, userID uuid.UUID, limit, offset int, unreadOnly bool) ([]*notification.Notification, error) {
	q := `
		SELECT id, target_user_id, source_user_id, type, payload, read, created_at, read_at
		FROM notifications
		WHERE target_user_id = $1`
	if unreadOnly {
		q += ` AND read = FALSE`
	}
	q += ` ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	rows, err := outbox.Conn(ctx, r.db).Query(ctx, q, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("notification_repo.GetByUser: %w", err)
	}
	defer rows.Close()

	var notifications []*notification.Notification
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, fmt.Errorf("notification_repo.GetByUser scan: %w", err)
		}
		notifications = append(notifications, n)
	}
	return notifications, rows.Err()
}

// MarkRead es idempotente: si ya estaba leída, la fila igual matchea (y read_at
// conserva el valor original), así que solo "no existe o no es tuya" da not found.
func (r *NotificationRepository) MarkRead(ctx context.Context, notificationID, userID uuid.UUID) error {
	const q = `
		UPDATE notifications
		SET read = TRUE, read_at = COALESCE(read_at, $1)
		WHERE id = $2 AND target_user_id = $3`

	tag, err := outbox.Conn(ctx, r.db).Exec(ctx, q, time.Now().UTC(), notificationID, userID)
	if err != nil {
		return fmt.Errorf("notification_repo.MarkRead: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return notification.ErrNotificationNotFound
	}
	return nil
}

func (r *NotificationRepository) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	const q = `
		UPDATE notifications
		SET read = TRUE, read_at = $1
		WHERE target_user_id = $2 AND read = FALSE`

	_, err := outbox.Conn(ctx, r.db).Exec(ctx, q, time.Now().UTC(), userID)
	if err != nil {
		return fmt.Errorf("notification_repo.MarkAllRead: %w", err)
	}
	return nil
}

func (r *NotificationRepository) CountUnread(ctx context.Context, userID uuid.UUID) (int, error) {
	const q = `SELECT COUNT(*) FROM notifications WHERE target_user_id = $1 AND read = FALSE`
	var count int
	if err := outbox.Conn(ctx, r.db).QueryRow(ctx, q, userID).Scan(&count); err != nil {
		return 0, fmt.Errorf("notification_repo.CountUnread: %w", err)
	}
	return count, nil
}

func (r *NotificationRepository) DeleteNewFollower(ctx context.Context, targetUserID, followerID uuid.UUID) (int64, error) {
	const q = `
		DELETE FROM notifications
		WHERE target_user_id = $1 AND source_user_id = $2 AND type = $3`
	tag, err := outbox.Conn(ctx, r.db).Exec(ctx, q, targetUserID, followerID, string(notification.TypeNewFollower))
	if err != nil {
		return 0, fmt.Errorf("notification_repo.DeleteNewFollower: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ── FollowStateRepository ─────────────────────────────────────────────────────

// FollowStateRepository implementa notification.FollowStateRepository sobre la
// tabla follow_states. Usa la tx del context si existe (outbox.Conn).
type FollowStateRepository struct {
	db *pgxpool.Pool
}

func NewFollowStateRepository(db *pgxpool.Pool) *FollowStateRepository {
	return &FollowStateRepository{db: db}
}

func (r *FollowStateRepository) ApplyFollow(ctx context.Context, followerID, followingID uuid.UUID, at time.Time) (bool, error) {
	return r.apply(ctx, followerID, followingID, true, at)
}

func (r *FollowStateRepository) ApplyUnfollow(ctx context.Context, followerID, followingID uuid.UUID, at time.Time) (bool, error) {
	return r.apply(ctx, followerID, followingID, false, at)
}

// apply guarda el estado solo si el evento es más nuevo que el último aplicado
// (estrictamente: una reentrega del mismo evento no vuelve a aplicarse).
func (r *FollowStateRepository) apply(ctx context.Context, followerID, followingID uuid.UUID, active bool, at time.Time) (bool, error) {
	const q = `
		INSERT INTO follow_states (follower_id, following_id, active, state_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (follower_id, following_id) DO UPDATE
		SET active = EXCLUDED.active, state_at = EXCLUDED.state_at
		WHERE follow_states.state_at < EXCLUDED.state_at`
	tag, err := outbox.Conn(ctx, r.db).Exec(ctx, q, followerID, followingID, active, at)
	if err != nil {
		return false, fmt.Errorf("follow_state_repo.apply: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func scanNotification(row pgx.Row) (*notification.Notification, error) {
	var n notification.Notification
	var notifType string
	var payloadRaw []byte

	err := row.Scan(
		&n.ID, &n.TargetUserID, &n.SourceUserID,
		&notifType, &payloadRaw,
		&n.Read, &n.CreatedAt, &n.ReadAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, notification.ErrNotificationNotFound
		}
		return nil, err
	}

	n.Type = notification.Type(notifType)
	if err := json.Unmarshal(payloadRaw, &n.Payload); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}
	return &n, nil
}

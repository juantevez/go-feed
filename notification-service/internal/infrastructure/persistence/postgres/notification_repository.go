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
)

// NotificationRepository implementa notification.Repository sobre Postgres.
type NotificationRepository struct {
	db *pgxpool.Pool
}

func NewNotificationRepository(db *pgxpool.Pool) *NotificationRepository {
	return &NotificationRepository{db: db}
}

func (r *NotificationRepository) Save(ctx context.Context, n *notification.Notification) error {
	payload, err := json.Marshal(n.Payload)
	if err != nil {
		return fmt.Errorf("notification_repo.Save: marshal payload: %w", err)
	}

	const q = `
		INSERT INTO notifications (id, target_user_id, source_user_id, type, payload, read, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`

	_, err = r.db.Exec(ctx, q,
		n.ID, n.TargetUserID, n.SourceUserID,
		string(n.Type), payload, n.Read, n.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("notification_repo.Save: %w", err)
	}
	return nil
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

	rows, err := r.db.Query(ctx, q, userID, limit, offset)
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

func (r *NotificationRepository) MarkRead(ctx context.Context, notificationID, userID uuid.UUID) error {
	const q = `
		UPDATE notifications
		SET read = TRUE, read_at = $1
		WHERE id = $2 AND target_user_id = $3 AND read = FALSE`

	tag, err := r.db.Exec(ctx, q, time.Now().UTC(), notificationID, userID)
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

	_, err := r.db.Exec(ctx, q, time.Now().UTC(), userID)
	if err != nil {
		return fmt.Errorf("notification_repo.MarkAllRead: %w", err)
	}
	return nil
}

func (r *NotificationRepository) CountUnread(ctx context.Context, userID uuid.UUID) (int, error) {
	const q = `SELECT COUNT(*) FROM notifications WHERE target_user_id = $1 AND read = FALSE`
	var count int
	if err := r.db.QueryRow(ctx, q, userID).Scan(&count); err != nil {
		return 0, fmt.Errorf("notification_repo.CountUnread: %w", err)
	}
	return count, nil
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

-- Migration: 0001_create_notifications.up.sql

CREATE TABLE IF NOT EXISTS notifications (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    target_user_id UUID        NOT NULL,   -- quien recibe la notificación
    source_user_id UUID        NOT NULL,   -- quien la genera
    type           VARCHAR(50) NOT NULL,   -- new_follower, new_post, post_liked, etc.
    payload        JSONB       NOT NULL DEFAULT '{}',
    read           BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    read_at        TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_notif_target_user
    ON notifications (target_user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_notif_unread
    ON notifications (target_user_id, read)
    WHERE read = FALSE;
    
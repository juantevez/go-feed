-- follow_states: último estado conocido de cada relación (seguidor → seguido), con
-- last-write-wins por timestamp del evento.
-- users.followed y users.unfollowed se consumen en subscriptions separadas y pueden
-- llegar desordenados: un unfollow procesado antes que su follow no encontraría la
-- notificación para borrar, y el follow tardío la crearía igual. Cada evento solo
-- se aplica si es más nuevo que state_at.
CREATE TABLE IF NOT EXISTS follow_states (
    follower_id  UUID        NOT NULL,
    following_id UUID        NOT NULL,
    active       BOOLEAN     NOT NULL,
    state_at     TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (follower_id, following_id)
);

-- Borrado de la notificación de un seguidor puntual en el unfollow.
CREATE INDEX IF NOT EXISTS idx_notif_target_source_type
    ON notifications (target_user_id, source_user_id, type);

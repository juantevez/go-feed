-- deleted_posts: tombstone de los posts borrados.
-- posts.created y posts.deleted se consumen en subscriptions separadas: si el
-- delete se procesa antes que el create (ej. el create se reintentó con backoff),
-- el create tardío encuentra el tombstone y no vuelve a meter el post en los feeds.
CREATE TABLE IF NOT EXISTS deleted_posts (
    post_id    UUID        PRIMARY KEY,
    deleted_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

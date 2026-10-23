-- followers pasa a ser last-write-wins por timestamp de evento.
-- users.followed y users.unfollowed se consumen en subscriptions paralelas y
-- pueden llegar desordenados (o reentregados): cada evento solo se aplica si es
-- más nuevo que state_at. Un unfollow deja la fila con active = FALSE (tombstone)
-- en vez de borrarla, para que un follow más viejo que llegue después no la reviva.
ALTER TABLE followers
    ADD COLUMN IF NOT EXISTS active   BOOLEAN     NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS state_at TIMESTAMPTZ NOT NULL DEFAULT '-infinity';

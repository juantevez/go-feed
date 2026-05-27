-- Migration: 0001_create_feed_tables.up.sql

-- feed_entries: timeline materializado por usuario.
-- Índice compuesto (user_id, score DESC, post_id DESC) soporta
-- el cursor-based pagination en O(log n).
CREATE TABLE IF NOT EXISTS feed_entries (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID        NOT NULL,
    post_id     UUID        NOT NULL,
    author_id   UUID        NOT NULL,
    score       FLOAT8      NOT NULL,       -- unix timestamp ms (cronológico) o ranking score
    inserted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, post_id)               -- idempotencia: ON CONFLICT DO NOTHING
);

CREATE INDEX IF NOT EXISTS idx_fe_user_score
    ON feed_entries (user_id, score DESC, post_id DESC);

CREATE INDEX IF NOT EXISTS idx_fe_post_id
    ON feed_entries (post_id);              -- para invalidación por post

CREATE INDEX IF NOT EXISTS idx_fe_author_id
    ON feed_entries (user_id, author_id);   -- para unfollow cleanup

-- followers: grafo social read-only del feed-service.
-- Poblado via eventos users.followed.v1 / users.unfollowed.v1 (fase 2).
-- En fase 1 se puede poblar manualmente para testing.
CREATE TABLE IF NOT EXISTS followers (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    follower_id  UUID        NOT NULL,      -- quien sigue
    following_id UUID        NOT NULL,      -- a quien sigue
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (follower_id, following_id)
);

CREATE INDEX IF NOT EXISTS idx_followers_following
    ON followers (following_id, last_seen_at DESC); -- GetActiveFollowers
    
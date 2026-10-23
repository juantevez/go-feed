-- Tabla principal del grafo social.
-- UNIQUE (follower_id, following_id) garantiza idempotencia en Save.
CREATE TABLE IF NOT EXISTS follows (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    follower_id  UUID        NOT NULL,
    following_id UUID        NOT NULL,
    mutual       BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (follower_id, following_id),
    CHECK (follower_id != following_id)  -- no auto-follow a nivel DB
);

-- Índice principal: GetFollowers (¿quién sigue a X?)
CREATE INDEX IF NOT EXISTS idx_follows_following
    ON follows (following_id, created_at DESC);

-- Índice secundario: GetFollowing (¿a quién sigue X?)
CREATE INDEX IF NOT EXISTS idx_follows_follower
    ON follows (follower_id, created_at DESC);

-- Índice para lookup de mutualidad
CREATE INDEX IF NOT EXISTS idx_follows_pair
    ON follows (follower_id, following_id);
    
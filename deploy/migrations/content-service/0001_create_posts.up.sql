CREATE TYPE post_status     AS ENUM ('draft', 'published', 'deleted');
CREATE TYPE post_visibility AS ENUM ('public', 'followers_only', 'private');

CREATE TABLE IF NOT EXISTS posts (
    id          UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    author_id   UUID            NOT NULL,
    caption     TEXT            NOT NULL DEFAULT '',
    status      post_status     NOT NULL DEFAULT 'draft',
    visibility  post_visibility NOT NULL DEFAULT 'public',
    media_urls  TEXT[]          NOT NULL DEFAULT '{}',
    tags        TEXT[]          NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_posts_author    ON posts (author_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_posts_status    ON posts (status);
CREATE INDEX IF NOT EXISTS idx_posts_tags      ON posts USING GIN (tags);
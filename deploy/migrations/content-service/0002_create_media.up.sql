-- Tabla de metadata de media (los archivos viven en S3).
-- Permite auditoría, re-generación de URLs y limpieza de huérfanos.
CREATE TABLE IF NOT EXISTS media (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id     UUID        NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    type        VARCHAR(10) NOT NULL,   -- 'image' | 'video'
    mime_type   VARCHAR(50) NOT NULL,
    size_bytes  BIGINT      NOT NULL,
    filename    TEXT        NOT NULL,
    s3_key      TEXT        NOT NULL UNIQUE,
    url         TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_media_post_id ON media (post_id);
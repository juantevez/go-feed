-- author_posts: índice local de posts por autor, alimentado por posts.created /
-- posts.deleted. Permite cargar los posts recientes de un autor en el feed de
-- quien empieza a seguirlo, sin consultar a content-service.
CREATE TABLE IF NOT EXISTS author_posts (
    post_id    UUID        PRIMARY KEY,
    author_id  UUID        NOT NULL,
    score      FLOAT8      NOT NULL,      -- mismo score que feed_entries (unix ms de publicación)
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_author_posts_author
    ON author_posts (author_id, score DESC);

-- Seed con los posts ya repartidos: desde que el fan-out incluye al autor,
-- todo post vivo tiene al menos una entrada en feed_entries.
INSERT INTO author_posts (post_id, author_id, score)
SELECT DISTINCT ON (post_id) post_id, author_id, score
FROM feed_entries
ORDER BY post_id
ON CONFLICT (post_id) DO NOTHING;

-- +goose Up
-- +goose StatementBegin

-- ============================================================================
-- FEED BACKEND - Esquema Inicial v1.0
-- PostgreSQL 16+ | UTF8 | UTC
-- ============================================================================

-- Extensiones requeridas
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";  -- Para búsquedas fuzzy en usernames

-- ============================================================================
-- 1. TABLA: users
-- ============================================================================
CREATE TABLE IF NOT EXISTS users (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username        VARCHAR(30) NOT NULL UNIQUE,
    email           VARCHAR(255) NOT NULL UNIQUE,
    password_hash   BYTEA NOT NULL,  -- bcrypt/argon2 hash
    full_name       VARCHAR(100),
    bio             TEXT,
    avatar_url      TEXT,
    website         TEXT,
    
    -- Contadores denormalizados (actualizados por triggers/workers)
    followers_count BIGINT NOT NULL DEFAULT 0,
    following_count BIGINT NOT NULL DEFAULT 0,
    posts_count     BIGINT NOT NULL DEFAULT 0,
    
    -- Metadata
    is_verified     BOOLEAN NOT NULL DEFAULT FALSE,
    is_private      BOOLEAN NOT NULL DEFAULT FALSE,
    is_banned       BOOLEAN NOT NULL DEFAULT FALSE,
    
    -- Timestamps
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at    TIMESTAMPTZ,
    
    -- Soft delete
    deleted_at      TIMESTAMPTZ,
    
    -- Constraints
    CONSTRAINT username_length CHECK (char_length(username) BETWEEN 3 AND 30),
    CONSTRAINT username_format CHECK (username ~ '^[a-zA-Z0-9_\.]+$')
);

-- Índices para queries frecuentes
CREATE INDEX idx_users_username_trgm ON users USING gin (username gin_trgm_ops);
CREATE INDEX idx_users_email ON users (email) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_created_at ON users (created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_last_seen ON users (last_seen_at DESC) WHERE last_seen_at IS NOT NULL;

-- Trigger para actualizar updated_at automáticamente
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- 2. TABLA: follows (grafo social)
-- ============================================================================
CREATE TABLE IF NOT EXISTS follows (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    follower_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    following_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    
    -- Metadata de la relación
    is_close_friend BOOLEAN NOT NULL DEFAULT FALSE,  -- Para ranking de feed
    notifications_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Unicidad: un follower no puede seguir al mismo usuario dos veces
    CONSTRAINT unique_follow_pair UNIQUE (follower_id, following_id),
    CONSTRAINT no_self_follow CHECK (follower_id != following_id)
);

-- Índices críticos para consultas de feed y perfil
CREATE INDEX idx_follows_follower ON follows (follower_id, created_at DESC);
CREATE INDEX idx_follows_following ON follows (following_id, created_at DESC);
CREATE INDEX idx_follows_mutual ON follows (follower_id, following_id) 
    WHERE is_close_friend = TRUE;

-- Trigger para actualizar contadores denormalizados en users
CREATE OR REPLACE FUNCTION update_follow_counters()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        UPDATE users SET followers_count = followers_count + 1 WHERE id = NEW.following_id;
        UPDATE users SET following_count = following_count + 1 WHERE id = NEW.follower_id;
    ELSIF TG_OP = 'DELETE' THEN
        UPDATE users SET followers_count = followers_count - 1 WHERE id = OLD.following_id;
        UPDATE users SET following_count = following_count - 1 WHERE id = OLD.follower_id;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_follows_counters
    AFTER INSERT OR DELETE ON follows
    FOR EACH ROW
    EXECUTE FUNCTION update_follow_counters();

-- ============================================================================
-- 3. TABLA: posts (contenido principal)
-- ============================================================================
CREATE TABLE IF NOT EXISTS posts (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    author_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    
    -- Contenido
    caption         TEXT,
    media_urls      JSONB NOT NULL DEFAULT '[]'::jsonb,  -- [{url, type, width, height}]
    
    -- Metadata
    visibility      VARCHAR(20) NOT NULL DEFAULT 'public' 
        CHECK (visibility IN ('public', 'followers_only', 'private')),
    location        JSONB,  -- {name, lat, lng}
    tags            TEXT[],  -- Hashtags extraídos del caption
    
    -- Contadores denormalizados
    likes_count     BIGINT NOT NULL DEFAULT 0,
    comments_count  BIGINT NOT NULL DEFAULT 0,
    shares_count    BIGINT NOT NULL DEFAULT 0,
    
    -- Timestamps
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Soft delete + moderación
    deleted_at      TIMESTAMPTZ,
    moderation_status VARCHAR(20) NOT NULL DEFAULT 'approved'
        CHECK (moderation_status IN ('pending', 'approved', 'rejected', 'removed')),
    
    -- Constraints
    CONSTRAINT caption_length CHECK (char_length(caption) <= 2200),
    CONSTRAINT media_max_count CHECK (jsonb_array_length(media_urls) <= 10)
);

-- Índices para feed y descubrimiento
CREATE INDEX idx_posts_author_created ON posts (author_id, created_at DESC) 
    WHERE deleted_at IS NULL AND moderation_status = 'approved';
CREATE INDEX idx_posts_created_global ON posts (created_at DESC) 
    WHERE deleted_at IS NULL AND moderation_status = 'approved' AND visibility = 'public';
CREATE INDEX idx_posts_tags ON posts USING gin (tags) 
    WHERE deleted_at IS NULL AND moderation_status = 'approved';
CREATE INDEX idx_posts_location ON posts USING gin (location) 
    WHERE location IS NOT NULL AND deleted_at IS NULL;

-- Trigger para updated_at y posts_count
CREATE TRIGGER trg_posts_updated_at
    BEFORE UPDATE ON posts
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE OR REPLACE FUNCTION update_posts_count()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' AND NEW.deleted_at IS NULL THEN
        UPDATE users SET posts_count = posts_count + 1 WHERE id = NEW.author_id;
    ELSIF TG_OP = 'UPDATE' AND OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL THEN
        UPDATE users SET posts_count = posts_count - 1 WHERE id = NEW.author_id;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_posts_count
    AFTER INSERT OR UPDATE OF deleted_at ON posts
    FOR EACH ROW
    EXECUTE FUNCTION update_posts_count();

-- ============================================================================
-- 4. TABLA: feed_entries (timeline materializado - fan-out)
-- ============================================================================
-- Nota: Esta tabla es opcional si usas Redis para el feed.
-- Se incluye como fallback DB y para analytics/auditoría.

CREATE TABLE IF NOT EXISTS feed_entries (
    id              BIGSERIAL PRIMARY KEY,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    post_id         UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    
    -- Score para ranking (calculado por worker)
    score           DOUBLE PRECISION NOT NULL DEFAULT 0,
    
    -- Metadata para filtrado
    author_id       UUID NOT NULL,  -- Denormalizado para evitar JOIN
    post_visibility VARCHAR(20) NOT NULL,
    
    -- Timestamps para cursor pagination
    inserted_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    post_created_at TIMESTAMPTZ NOT NULL,
    
    -- Unicidad: un post aparece una vez por usuario en el feed
    CONSTRAINT unique_user_post UNIQUE (user_id, post_id)
);

-- Índices CRÍTICOS para paginación cursor-based
-- Formato de cursor: (post_created_at, post_id) para orden estable
CREATE INDEX idx_feed_entries_user_cursor ON feed_entries 
    (user_id, post_created_at DESC, post_id DESC)
    WHERE post_visibility IN ('public', 'followers_only');

-- Índice para limpieza de feeds antiguos
CREATE INDEX idx_feed_entries_inserted ON feed_entries (inserted_at DESC);

-- Función para limpiar entries antiguos (llamar por cron job)
CREATE OR REPLACE FUNCTION cleanup_old_feed_entries(max_age_days INTEGER DEFAULT 30)
RETURNS TABLE(deleted_count BIGINT) AS $$
BEGIN
    RETURN QUERY
    DELETE FROM feed_entries 
    WHERE inserted_at < NOW() - (max_age_days || ' days')::INTERVAL
    RETURNING COUNT(*) OVER ();
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- 5. TABLA: interactions (likes, comments, shares)
-- ============================================================================
CREATE TABLE IF NOT EXISTS interactions (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    post_id         UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    
    -- Tipo de interacción
    type            VARCHAR(20) NOT NULL 
        CHECK (type IN ('like', 'comment', 'share', 'save')),
    
    -- Payload específico por tipo (JSONB flexible)
    payload         JSONB DEFAULT '{}'::jsonb,
    -- Ejemplos:
    -- like:   {}
    -- comment: {"text": "...", "parent_id": "uuid"}
    -- share:   {"via": "story", "caption": "..."}
    
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Unicidad: un usuario solo puede dar like una vez por post
    CONSTRAINT unique_user_post_like UNIQUE (user_id, post_id, type)
        WHERE type = 'like'
);

-- Índices para consultas de engagement
CREATE INDEX idx_interactions_post_type ON interactions (post_id, type, created_at DESC);
CREATE INDEX idx_interactions_user ON interactions (user_id, created_at DESC);
CREATE INDEX idx_interactions_payload_gin ON interactions USING gin (payload)
    WHERE payload IS NOT NULL AND payload != '{}'::jsonb;

-- Trigger para actualizar contadores en posts
CREATE OR REPLACE FUNCTION update_post_interaction_counters()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.type = 'like' THEN
            UPDATE posts SET likes_count = likes_count + 1 WHERE id = NEW.post_id;
        ELSIF NEW.type = 'comment' THEN
            UPDATE posts SET comments_count = comments_count + 1 WHERE id = NEW.post_id;
        ELSIF NEW.type = 'share' THEN
            UPDATE posts SET shares_count = shares_count + 1 WHERE id = NEW.post_id;
        END IF;
    ELSIF TG_OP = 'DELETE' THEN
        IF OLD.type = 'like' THEN
            UPDATE posts SET likes_count = likes_count - 1 WHERE id = OLD.post_id;
        ELSIF OLD.type = 'comment' THEN
            UPDATE posts SET comments_count = comments_count - 1 WHERE id = OLD.post_id;
        ELSIF OLD.type = 'share' THEN
            UPDATE posts SET shares_count = shares_count - 1 WHERE id = OLD.post_id;
        END IF;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_interactions_counters
    AFTER INSERT OR DELETE ON interactions
    FOR EACH ROW
    EXECUTE FUNCTION update_post_interaction_counters();

-- ============================================================================
-- 6. TABLA: notifications (alertas en tiempo real)
-- ============================================================================
CREATE TABLE IF NOT EXISTS notifications (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    target_user_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    
    -- Origen de la notificación
    source_user_id  UUID REFERENCES users(id) ON DELETE SET NULL,
    post_id         UUID REFERENCES posts(id) ON DELETE SET NULL,
    interaction_id  UUID REFERENCES interactions(id) ON DELETE SET NULL,
    
    -- Tipo y contenido
    type            VARCHAR(30) NOT NULL 
        CHECK (type IN ('follow', 'like', 'comment', 'mention', 'reply', 'system')),
    title           VARCHAR(255),
    body            TEXT,
    
    -- Estado
    is_read         BOOLEAN NOT NULL DEFAULT FALSE,
    delivered_at    TIMESTAMPTZ,  -- NULL = pendiente de entrega
    
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL DEFAULT (NOW() + INTERVAL '7 days')
);

-- Índices para bandeja de notificaciones
CREATE INDEX idx_notifications_user_unread ON notifications 
    (target_user_id, created_at DESC) WHERE is_read = FALSE;
CREATE INDEX idx_notifications_user_all ON notifications 
    (target_user_id, created_at DESC);
CREATE INDEX idx_notifications_pending_delivery ON notifications 
    (delivered_at) WHERE delivered_at IS NULL AND expires_at > NOW();

-- Función para limpieza automática de notificaciones expiradas
CREATE OR REPLACE FUNCTION cleanup_expired_notifications()
RETURNS TABLE(deleted_count BIGINT) AS $$
BEGIN
    RETURN QUERY
    DELETE FROM notifications 
    WHERE expires_at < NOW()
    RETURNING COUNT(*) OVER ();
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- 7. TABLA: idempotency_keys (auditoría / fallback de Redis)
-- ============================================================================
-- Nota: El store principal es Redis. Esta tabla es para auditoría y recovery.

CREATE TABLE IF NOT EXISTS idempotency_keys (
    id              BIGSERIAL PRIMARY KEY,
    key             VARCHAR(128) NOT NULL UNIQUE,
    
    -- Estado de la operación
    status          VARCHAR(20) NOT NULL 
        CHECK (status IN ('pending', 'completed', 'failed', 'expired')),
    
    -- Metadata de la request
    method          VARCHAR(10) NOT NULL,
    path            VARCHAR(255) NOT NULL,
    user_id         UUID REFERENCES users(id) ON DELETE SET NULL,
    
    -- Respuesta cacheada (solo para status=completed)
    response_code   INTEGER,
    response_headers JSONB,
    response_body   BYTEA,
    
    -- Error (solo para status=failed)
    error_message   TEXT,
    
    -- Timestamps
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ,
    expires_at      TIMESTAMPTZ NOT NULL,
    
    -- Índice para cleanup
    CONSTRAINT valid_expiration CHECK (expires_at > created_at)
);

CREATE INDEX idx_idempotency_keys_expiry ON idempotency_keys (expires_at) 
    WHERE status != 'expired';
CREATE INDEX idx_idempotency_keys_user ON idempotency_keys (user_id, created_at DESC);

-- ============================================================================
-- 8. VISTAS MATERIALIZADAS (opcional, para analytics)
-- ============================================================================

-- Vista para métricas de usuario (actualizar por job horario)
CREATE MATERIALIZED VIEW IF NOT EXISTS user_metrics_summary AS
SELECT 
    u.id,
    u.username,
    u.followers_count,
    u.following_count,
    u.posts_count,
    COUNT(DISTINCT CASE WHEN i.type = 'like' THEN i.id END) as total_likes_received,
    COUNT(DISTINCT CASE WHEN i.type = 'comment' THEN i.id END) as total_comments_received,
    AVG(EXTRACT(EPOCH FROM (NOW() - p.created_at))/3600) as avg_post_age_hours
FROM users u
LEFT JOIN posts p ON p.author_id = u.id AND p.deleted_at IS NULL
LEFT JOIN interactions i ON i.post_id = p.id AND i.type IN ('like', 'comment')
WHERE u.deleted_at IS NULL
GROUP BY u.id, u.username, u.followers_count, u.following_count, u.posts_count;

CREATE UNIQUE INDEX idx_user_metrics_summary_id ON user_metrics_summary (id);

-- ============================================================================
-- 9. FUNCIONES UTILITARIAS
-- ============================================================================

-- Función para generar cursor de paginación estable
CREATE OR REPLACE FUNCTION encode_cursor(created_at TIMESTAMPTZ, id UUID)
RETURNS TEXT AS $$
BEGIN
    RETURN encode(concat(extract(epoch from created_at)::text, '_', id::text)::bytea, 'base64');
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- Función para decodificar cursor
CREATE OR REPLACE FUNCTION decode_cursor(cursor_text TEXT)
RETURNS TABLE(created_at TIMESTAMPTZ, id UUID) AS $$
DECLARE
    decoded TEXT;
    parts TEXT[];
BEGIN
    decoded := convert_from(decode(cursor_text, 'base64'), 'UTF8');
    parts := string_to_array(decoded, '_');
    created_at := to_timestamp(parts[1]::DOUBLE PRECISION);
    id := parts[2]::UUID;
    RETURN NEXT;
END;
$$ LANGUAGE plpgsql;

-- Función para validar permisos de visibilidad de posts
CREATE OR REPLACE FUNCTION can_view_post(
    viewer_id UUID, 
    post_author_id UUID, 
    post_visibility VARCHAR(20)
)
RETURNS BOOLEAN AS $$
BEGIN
    IF post_visibility = 'public' THEN
        RETURN TRUE;
    ELSIF post_visibility = 'followers_only' THEN
        RETURN EXISTS (
            SELECT 1 FROM follows 
            WHERE follower_id = viewer_id 
            AND following_id = post_author_id
        );
    ELSIF post_visibility = 'private' THEN
        RETURN viewer_id = post_author_id;
    END IF;
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- 10. ROLES Y PERMISOS (ajustar según tu estrategia de DB access)
-- ============================================================================

-- Rol para la aplicación (lectura/escritura limitada)
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'feed_app') THEN
        CREATE ROLE feed_app WITH LOGIN PASSWORD 'change_me_in_prod';
    END IF;
END $$;

-- Grants mínimos necesarios
GRANT CONNECT ON DATABASE feed_db TO feed_app;
GRANT USAGE ON SCHEMA public TO feed_app;

-- Tablas: SELECT, INSERT, UPDATE (no DELETE directo, usar soft delete)
GRANT SELECT, INSERT, UPDATE ON 
    users, follows, posts, feed_entries, interactions, notifications, idempotency_keys 
    TO feed_app;

-- Secuencias: para auto-incrementos
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO feed_app;

-- Restringir DELETE directo (forzar uso de deleted_at)
-- (Implementar con Row Level Security si se requiere)

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- (Opcional: scripts de rollback - generalmente no se usan en prod)
DROP MATERIALIZED VIEW IF EXISTS user_metrics_summary;
DROP FUNCTION IF EXISTS cleanup_expired_notifications, cleanup_old_feed_entries, 
    encode_cursor, decode_cursor, can_view_post, update_follow_counters, 
    update_posts_count, update_post_interaction_counters, update_updated_at_column;
DROP TABLE IF EXISTS 
    idempotency_keys, notifications, interactions, feed_entries, 
    posts, follows, users CASCADE;
DROP EXTENSION IF EXISTS "pg_trgm", "uuid-ossp";
-- +goose StatementEnd

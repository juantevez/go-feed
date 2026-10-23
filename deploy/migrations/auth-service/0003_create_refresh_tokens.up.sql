-- Migration: 0003_create_refresh_tokens.up.sql
-- Tracks refresh tokens for rotation and revocation.
-- Access tokens are stateless (validated via JWT signature);
-- only refresh tokens need persistence.

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    jti        VARCHAR(36) NOT NULL UNIQUE,   -- JWT ID claim
    expires_at TIMESTAMPTZ NOT NULL,
    revoked    BOOLEAN     NOT NULL DEFAULT FALSE,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_rt_user_id   ON refresh_tokens (user_id);
CREATE INDEX IF NOT EXISTS idx_rt_jti       ON refresh_tokens (jti);
CREATE INDEX IF NOT EXISTS idx_rt_expires   ON refresh_tokens (expires_at);

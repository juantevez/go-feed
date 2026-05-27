-- Migration: 0002_create_idempotency_keys.up.sql
-- Implements the idempotency store defined in the functional design.

CREATE TYPE idemp_status AS ENUM ('PENDING', 'COMPLETED', 'FAILED', 'EXPIRED');

CREATE TABLE IF NOT EXISTS idempotency_keys (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    idemp_key       VARCHAR(255) NOT NULL UNIQUE,
    status          idemp_status NOT NULL DEFAULT 'PENDING',
    response_body   TEXT,                   -- serialised response cached on COMPLETED
    response_status INT,                    -- HTTP status code cached on COMPLETED
    action          VARCHAR(100) NOT NULL,  -- e.g. "posts:create"
    source          VARCHAR(100),           -- client_id or worker_id
    trace_id        VARCHAR(100),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ  NOT NULL,
    resolved_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_idemp_key    ON idempotency_keys (idemp_key);
CREATE INDEX IF NOT EXISTS idx_idemp_status ON idempotency_keys (status);
CREATE INDEX IF NOT EXISTS idx_idemp_expires ON idempotency_keys (expires_at);

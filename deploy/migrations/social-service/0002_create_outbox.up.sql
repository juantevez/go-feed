-- Transactional outbox: los eventos se insertan en la misma tx que el cambio
-- de negocio y un relay los publica a NATS JetStream (shared/outbox).
CREATE TABLE IF NOT EXISTS outbox (
    id           UUID        PRIMARY KEY,           -- = event_id, usado como Nats-Msg-Id
    topic        TEXT        NOT NULL,
    payload      JSONB       NOT NULL,              -- events.Envelope serializado
    created_at   TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    published_at TIMESTAMPTZ,
    attempts     INT         NOT NULL DEFAULT 0,
    last_error   TEXT
);

-- El relay solo lee pendientes, en orden de creación.
CREATE INDEX IF NOT EXISTS idx_outbox_pending
    ON outbox (created_at, id)
    WHERE published_at IS NULL;

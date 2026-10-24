-- Contexto de OpenTelemetry (traceparent, tracestate, baggage) de la request
-- que escribió el evento. El relay lo pone en los headers de NATS y el
-- consumer continúa el mismo trace (shared/telemetry). NULL = sin trace.
ALTER TABLE outbox ADD COLUMN IF NOT EXISTS trace_context JSONB;

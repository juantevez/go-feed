-- Idempotencia ante reentregas de JetStream: una notificación por (evento, destinatario).
-- event_id es NULL para notificaciones que no nacen de un evento del bus.
-- Incluye target_user_id porque un mismo evento puede notificar a varios usuarios
-- (ej. posts.created → cada seguidor del autor).
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS event_id UUID;

CREATE UNIQUE INDEX IF NOT EXISTS uq_notif_event_target
    ON notifications (event_id, target_user_id)
    WHERE event_id IS NOT NULL;

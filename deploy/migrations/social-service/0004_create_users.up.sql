-- users: directorio local de usuarios, alimentado por auth.user.registered.v1.
-- Sirve para rechazar follows a usuarios que no existen sin consultar a
-- auth-service. Solo lo necesario: el email no se copia.
-- Los usuarios registrados antes de esta migración se cargan con
-- deploy/scripts/backfill-social-users.sh (el stream AUTH retiene 7 días).
CREATE TABLE IF NOT EXISTS users (
    id            UUID        PRIMARY KEY,
    username      TEXT        NOT NULL,
    registered_at TIMESTAMPTZ NOT NULL
);

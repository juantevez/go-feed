#!/usr/bin/env sh
# Carga en el directorio de usuarios de social-service (ig_social.users) los
# usuarios ya registrados en auth-service. Hace falta una sola vez, al aplicar la
# migración 0004 de social-service: los usuarios nuevos llegan solos por el
# evento auth.user.registered.v1, pero el stream AUTH retiene solo 7 días.
#
# Idempotente: los que ya estén se saltean. Usa los contenedores del compose.
#   deploy/scripts/backfill-social-users.sh
set -eu

AUTH_DB_USER=${AUTH_DB_USER:-ig_user}
AUTH_DB_NAME=${AUTH_DB_NAME:-ig_auth}
SOCIAL_DB_USER=${SOCIAL_DB_USER:-ig_user}
SOCIAL_DB_NAME=${SOCIAL_DB_NAME:-ig_social}

# Un solo script por stdin (misma sesión, para la tabla temporal): psql -c no
# acepta un COPY FROM STDIN en medio de varias sentencias.
{
  echo "CREATE TEMP TABLE incoming (id UUID, username TEXT, registered_at TIMESTAMPTZ);"
  echo "COPY incoming FROM STDIN;"
  docker exec my-ig-postgres-auth psql -U "$AUTH_DB_USER" -d "$AUTH_DB_NAME" -qAt \
    -c "COPY (SELECT id, username, created_at FROM users) TO STDOUT"
  echo '\.'
  echo "WITH ins AS (
          INSERT INTO users (id, username, registered_at)
          SELECT id, username, registered_at FROM incoming
          ON CONFLICT (id) DO NOTHING
          RETURNING 1
        )
        SELECT 'usuarios agregados: ' || count(*) FROM ins;"
} | docker exec -i my-ig-postgres-social psql -U "$SOCIAL_DB_USER" -d "$SOCIAL_DB_NAME" -qAt -v ON_ERROR_STOP=1

# auth-service

Bounded context de **identidad y autenticación**: registro de usuarios, login, emisión y rotación
de tokens JWT. Es el único servicio que conoce credenciales; el resto valida los access tokens con
el secreto compartido y nunca consulta a auth-service.

| | |
|---|---|
| Puerto | `8080` |
| Base de datos | `postgres-auth` → `ig_auth` |
| Código | [`services/auth-service/`](../../services/auth-service/) |
| Migraciones | [`deploy/migrations/auth-service/`](../../deploy/migrations/auth-service/) |
| Eventos que publica | `auth.user.registered.v1`, `auth.user.logged_in.v1` |
| Eventos que consume | — |

---

## Arquitectura

Hexagonal: el dominio y la aplicación no importan nada de infraestructura.

```
services/auth-service/
├── cmd/server/main.go                         # wiring: DB, NATS, outbox relay, HTTP
├── configs/config.go                          # config desde env vars
└── internal/
    ├── domain/
    │   ├── user/        user.go               # agregado User + invariantes
    │   │                events.go             # builders de Envelope (registered, logged_in)
    │   │                repository.go         # puerto user.Repository
    │   └── token/       token.go              # entidad Token (refresh), Kind, errores
    │                    service.go            # puerto token.Service (JWT)
    ├── application/
    │   ├── port/input/  auth_usecase.go       # puerto driving: Register, Login, Refresh, Logout
    │   ├── port/output/ event_publisher.go    # puerto driven: EventPublisher
    │   │                tx_manager.go         # puerto driven: TxManager
    │   └── usecase/     auth_service.go       # implementación de los casos de uso
    └── infrastructure/
        ├── http/        handler/ router/ middleware/   # adaptador HTTP (chi)
        ├── jwt/         jwt_service.go        # adaptador token.Service (HS256)
        └── persistence/postgres/              # UserRepository, RefreshTokenRepository
```

Dependencias de `shared/`:
- `shared/events`: `Envelope`, tópicos y payloads (`UserRegisteredPayload`, `UserLoggedInPayload`).
- `shared/outbox`: `TxManager`, `Publisher` (escribe en la tabla `outbox`) y `Relay` (publica a JetStream).
- `shared/revocation`: `Denylist`, la lista de access tokens revocados por logout (bucket KV de NATS).

---

## Modelo de dominio

### `User` (agregado raíz)

| Campo | Regla |
|---|---|
| `ID` | UUID generado en `user.New` |
| `Username` | `^[a-zA-Z0-9_]{3,50}$`, se recortan espacios. Único en DB. |
| `Email` | Se normaliza con `user.NormalizeEmail` (minúsculas, sin espacios) al registrar **y al buscar** en login, así `Juan@X.com` y `juan@x.com` son la misma cuenta. Formato validado por regex. Único en DB. |
| `PasswordHash` | bcrypt con costo 12. El dominio recibe el hash, nunca la contraseña. |
| `Role` | `user` (default) o `admin`. Viaja en el claim `role` del JWT. |
| `IsVerified` | Siempre `false`: no hay flujo de verificación de email todavía. |

La contraseña en texto plano solo existe en el handler y en `AuthService.Register` (para hashearla).
El handler exige al menos 8 caracteres.

Las violaciones de invariantes se devuelven como `*user.ValidationError{Field, Msg}`: el handler
las traduce a `400` con `Msg` como mensaje.

### `Token`

Representa un **refresh token** persistido para rotación y revocación (`Kind = refresh`).
Los access tokens no se persisten: son stateless y se validan solo por firma y expiración.

---

## API HTTP

Todas las respuestas de error tienen la forma `{"error": "<mensaje>"}`.

| Método | Ruta | Auth | Éxito |
|---|---|---|---|
| `POST` | `/auth/register` | — | `201 {"user_id"}` |
| `POST` | `/auth/login` | — | `200 {"access_token", "refresh_token"}` |
| `POST` | `/auth/refresh` | — | `200 {"access_token", "refresh_token"}` |
| `POST` | `/auth/logout` | Bearer (access) | `204` |
| `GET` | `/health`, `/ready` | — | `200` (sin body) |

La especificación completa está en [`api/openapi/specs.yaml`](../../api/openapi/specs.yaml) y hay
ejemplos en la colección de Postman ([`api/postman/`](../../api/postman/)).

### `POST /auth/register`

```json
{ "username": "juan_dev", "email": "juan@example.com", "password": "supersecret123" }
```

| Status | Cuándo |
|---|---|
| `201` | Usuario creado. Se encola `auth.user.registered.v1` en la misma transacción. |
| `400` | Body inválido, campos vacíos, contraseña de menos de 8 caracteres, o username/email con formato inválido. El `error` dice el motivo (ej. `"email format is invalid"`). |
| `409` | Email o username ya registrados (ver [limitaciones](#limitaciones-conocidas)). |
| `500` | Error inesperado. |

### `POST /auth/login`

```json
{ "email": "juan@example.com", "password": "supersecret123" }
```

| Status | Cuándo |
|---|---|
| `200` | Credenciales válidas. Se persiste el refresh token y se encola `auth.user.logged_in.v1`, ambos en una transacción. |
| `400` | Body inválido o campos vacíos. |
| `401` | `invalid credentials`: mismo mensaje **y mismo tiempo de respuesta** si el email no existe o si la contraseña es incorrecta. |
| `500` | Error inesperado (ej. Postgres caído). No se reporta como credenciales inválidas. |

Cuando el email no existe, `Login` igual hace un `bcrypt.Compare` contra un hash de relleno con el
mismo costo. Sin eso, esa respuesta tardaba ~2 ms contra ~550 ms de una contraseña incorrecta, y
el tiempo revelaba qué emails están registrados.

### `POST /auth/refresh`

```json
{ "refresh_token": "<jwt>" }
```

Rota el par: revoca el refresh token recibido y emite uno nuevo junto con un access token nuevo.
Cada refresh token sirve **una sola vez**.

| Status | Cuándo |
|---|---|
| `200` | Par nuevo. |
| `400` | Body inválido o `refresh_token` vacío. |
| `401` | Token con firma inválida, expirado, revocado, ya usado, o que no es de tipo `refresh`. |

### `POST /auth/logout`

Requiere `Authorization: Bearer <access_token>`. Sin body. Cierra la sesión de ese token:

1. Revoca el refresh token emitido en el mismo par (lo encuentra por `refresh_tokens.access_jti`),
   así ya no sirve para pedir tokens nuevos.
2. Agrega el `jti` del access token a la lista de revocados hasta su `exp`: a partir de ahí
   **ningún servicio lo acepta** (`401 "token revoked"`).

Las otras sesiones del mismo usuario (otros logins) siguen activas.

| Status | Cuándo |
|---|---|
| `204` | Sesión cerrada. |
| `401` | Sin token, token inválido, expirado o ya revocado (un segundo logout con el mismo token da `401`). |
| `500` | No se pudo revocar (Postgres o NATS caídos). El cliente puede reintentar: es idempotente. |

---

## Tokens JWT

Firmados con **HS256** usando `JWT_SECRET`. Access y refresh comparten el formato de claims y se
distinguen por `kind`:

```json
{
  "sub":  "8f2fd640-8fdf-4548-be45-1a3e5b54c43f",
  "jti":  "7813d299-39be-402c-9b6f-030b871cf63f",
  "iss":  "auth-service",
  "iat":  1792724065,
  "exp":  1792724965,
  "role": "user",
  "kind": "access"
}
```

| | Access token | Refresh token |
|---|---|---|
| TTL | `JWT_ACCESS_TTL` (default `15m`) | `JWT_REFRESH_TTL` (default `168h` = 7 días) |
| Persistido | No (solo su `jti`, en la fila del refresh del mismo par) | Sí, en `refresh_tokens` (por `jti`) |
| Validación | Firma + `exp` + `kind = access` + no está en la lista de revocados | Firma + `exp` + `kind = refresh` + existe en DB, no revocado ni expirado |
| Uso | Header `Authorization: Bearer` en todos los servicios | Solo en `POST /auth/refresh` |

### Flujo login → refresh

```mermaid
sequenceDiagram
    participant C as Cliente
    participant A as auth-service
    participant DB as postgres-auth

    C->>A: POST /auth/login {email, password}
    A->>DB: SELECT user (por email)
    A->>A: bcrypt.Compare
    A->>DB: BEGIN; INSERT refresh_tokens; INSERT outbox (logged_in); COMMIT
    A-->>C: {access_token, refresh_token}

    Note over C: el access token expira a los 15 min

    C->>A: POST /auth/refresh {refresh_token}
    A->>DB: SELECT refresh_tokens WHERE jti (¿existe? ¿revocado? ¿expirado?)
    A->>DB: UPDATE refresh_tokens SET revoked = TRUE WHERE jti AND revoked = FALSE
    A->>DB: INSERT refresh_tokens (nuevo jti)
    A-->>C: {access_token, refresh_token} nuevos
```

El `UPDATE ... WHERE revoked = FALSE` protege contra dos refresh concurrentes con el mismo token:
solo uno revoca la fila, y el otro recibe `401`.

### Revocación (logout)

Los access tokens son stateless y los valida cada servicio por su cuenta, así que la revocación
tiene que llegar a todos. Se resuelve con `shared/revocation`:

- **Almacenamiento:** bucket KV de NATS JetStream `REVOKED_TOKENS` (clave = `jti`, valor = `exp`
  del token). El bucket descarta entradas a las 24 h, más que cualquier TTL de access token.
- **Escritura:** solo auth-service, en el logout.
- **Lectura:** cada servicio mantiene una copia en memoria que se actualiza por `Watch`, así que
  validar un token no agrega ningún viaje por red. Al arrancar, el servicio carga todas las
  revocaciones vigentes antes de aceptar requests: un reinicio no "olvida" un logout.
- **Propagación:** milisegundos (lo que tarda NATS en notificar el cambio).

### Validación en los otros servicios

content-service, social-service, notification-service y feed-service tienen su propio
`JWTMiddleware`, que valida firma, expiración, `kind = access` y la lista de revocados, con el
mismo `JWT_SECRET`. Ninguno llama a auth-service. Consecuencias:

- `JWT_SECRET` tiene que ser **el mismo en todos los servicios**. En el compose el default es
  `changeme_in_production`.
- Rotar el secreto invalida todos los tokens emitidos.

feed-service toma el usuario del claim `sub`: `GET /feed` devuelve siempre el feed del dueño del
token (el header `X-User-ID` que usaba antes ya no se acepta).

---

## Persistencia

| Tabla | Uso |
|---|---|
| `users` | Agregado `User`. `UNIQUE (username)`, `UNIQUE (email)`. |
| `refresh_tokens` | Un registro por refresh token emitido: `jti` único, `access_jti` (el access token del mismo par, para el logout), `expires_at`, `revoked`, `revoked_at`. FK a `users` con `ON DELETE CASCADE`. |
| `outbox` | Eventos pendientes de publicar (ver abajo). |
| `idempotency_keys` | Creada por la migración `0002`, **sin uso en el código** (ver limitaciones). |

Los repositorios usan `outbox.Conn(ctx, pool)`: si hay una transacción en el `ctx` la usan, si no,
el pool. Así participan de `TxManager.WithinTx` sin cambiar sus firmas.

Migraciones (golang-migrate, las corre el contenedor `migrate-auth` al levantar el stack):

| Versión | Contenido |
|---|---|
| `0001` | `users` + extensión `pgcrypto` |
| `0002` | `idempotency_keys` + enum `idemp_status` |
| `0003` | `refresh_tokens` |
| `0004` | `outbox` |
| `0005` | `refresh_tokens.access_jti` + índice (los tokens emitidos antes quedan con `NULL`) |

---

## Eventos

Se publican con **transactional outbox** (`shared/outbox`): el evento se inserta en la tabla
`outbox` en la misma transacción que el cambio de negocio, y un relay en background lo publica a
JetStream con `Nats-Msg-Id = event_id` (dedupe del lado del stream). Si NATS está caído, la
operación HTTP igual responde bien y el evento sale cuando NATS vuelve.

Todos los eventos usan el `events.Envelope` estándar (`event_id`, `event_type`, `version`,
`timestamp`, `source_service`, `idempotency_key`, `trace_id`, `payload`). El `trace_id` es el
request ID de chi.

| Tópico | Stream | Cuándo | Payload | `idempotency_key` |
|---|---|---|---|---|
| `auth.user.registered.v1` | `AUTH` | Registro exitoso | `user_id`, `username`, `email`, `role`, `occured_at` | `register:<user_id>` |
| `auth.user.logged_in.v1` | `AUTH` | Login exitoso | `user_id`, `occured_at` | — |

Hoy **ningún servicio consume** estos eventos. `shared/events/topics.go` lista como consumidores
previstos a feed-service y notification-service (registered) y a analytics (logged_in).

Los payloads nunca incluyen el hash de la contraseña ni tokens.

---

## Configuración

| Variable | Requerida | Default | Descripción |
|---|---|---|---|
| `JWT_SECRET` | Sí | — | Secreto HMAC. Debe coincidir con content, social y notification. |
| `DB_DSN` | Sí | — | DSN de Postgres. |
| `NATS_URL` | Sí | — | URL de NATS. |
| `PORT` | No | `8080` | Puerto HTTP. |
| `JWT_ACCESS_TTL` | No | `15m` | Duración en formato Go (`time.ParseDuration`). |
| `JWT_REFRESH_TTL` | No | `168h` | Ídem. |
| `HTTP_READ_TIMEOUT` / `HTTP_WRITE_TIMEOUT` | No | `10s` | |
| `HTTP_IDLE_TIMEOUT` | No | `60s` | |
| `DB_CONNECT_TIMEOUT` / `NATS_CONNECT_TIMEOUT` | No | `5s` | |
| `LOG_LEVEL` | No | `info` | Se lee pero hoy no cambia el nivel del logger. |

Si falta alguna variable requerida, el servicio no arranca y lista todas las que faltan juntas.
Una duración con formato inválido cae al default **sin avisar**.

El pool de Postgres usa 10 conexiones máximas y 2 mínimas (fijo en código). La conexión a NATS
reintenta para siempre (`MaxReconnects(-1)`), porque el relay del outbox depende de ella.

---

## Operación

```bash
docker compose up -d auth-service     # desde la raíz (el .env define COMPOSE_FILE)
make logs-auth                        # logs JSON (slog)
make test                             # incluye los tests de auth-service
```

- **Logs:** JSON por stdout. Cada línea de un request lleva `request_id`. Nunca se loguean
  contraseñas ni tokens; sí el email en los intentos de login/registro y el `jti` en logout.
- **Graceful shutdown:** con `SIGINT`/`SIGTERM` deja de aceptar requests (timeout de 15 s) y
  después detiene el relay. Los eventos pendientes quedan en `outbox` y se publican al reiniciar.
- **Tests:** [`auth_service_register_test.go`](../../services/auth-service/internal/application/usecase/auth_service_register_test.go)
  cubre `Register` (éxito, email tomado, username inválido, falla del repo y falla al encolar el
  evento) y `Login` (email inexistente con el mismo costo que una contraseña incorrecta, error de
  la DB) con mocks de los puertos. No hay tests de `Refresh`, `Logout` ni del adaptador JWT.

---

## Limitaciones conocidas

Verificadas contra el servicio corriendo. Ordenadas por impacto.



1. **Username repetido devuelve `409 "email already registered"`.** Cualquier violación de
   `UNIQUE` se mapea a `ErrEmailAlreadyTaken`, así que el mensaje es engañoso cuando lo repetido
   es el username.

2. **`Refresh` no es atómico.** La revocación del token viejo y el insert del nuevo no van en una
   transacción: si el insert falla, el token viejo queda revocado y el cliente tiene que volver a
   hacer login.

3. **No hay detección de reuso de refresh tokens.** Reusar un refresh token ya rotado da `401`,
   pero no revoca la familia de tokens de ese usuario. Esa revocación es la práctica estándar
   para detectar un token robado; `RevokeAllForUser` ya existe en el repositorio y no se usa.

4. **Código y tablas sin uso:**
   - La tabla `idempotency_keys` (migración `0002`) no la usa nadie.
   - `RefreshTokenRepository.DeleteExpired` no se llama: `refresh_tokens` crece sin límite.
   - `IsVerified` nunca pasa a `true`.

5. **Secreto simétrico compartido.** Con HS256, cualquier servicio que valida tokens también
   puede emitirlos. RS256/EdDSA (auth firma con la clave privada, el resto valida con la pública)
   lo evitaría; el comentario de `jwt_service.go` ya lo menciona.

6. **CORS fijo a `http://localhost:5173`.** El front usa el proxy de Vite/nginx (mismo origen),
    así que hoy no molesta, pero cualquier otro origen que llame directo al servicio es rechazado.

7. **El logout depende de NATS.** Si el bucket KV no está disponible, el logout responde `500`
    y el access token sigue valiendo hasta que el cliente reintente. La revocación del refresh
    token en Postgres sí se aplica, porque va primero.

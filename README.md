# Instagram-style backend (Go + Hexagonal)

Backend de red social con modelo Pub/Sub, implementado en **Go 1.25** con arquitectura hexagonal y DDD.

## Stack

| Capa | Tecnología |
|------|-----------|
| API | Go 1.25 + chi |
| DB | PostgreSQL 16 (una instancia por servicio) |
| Mensajería | NATS 2.10 JetStream |
| Almacenamiento | MinIO (S3-compatible) |
| Migraciones | golang-migrate |
| Contenedores | Docker + Compose |

## Servicios

| Servicio | Puerto | Base de datos | Responsabilidad |
|----------|--------|---------------|-----------------|
| auth-service | 8080 | postgres-auth (5432) | Registro, login y tokens JWT |
| feed-service | 8081 | postgres-feed (5433) | Fan-out y consulta de feed |
| content-service | 8082 | postgres-content (5434) | Posts y media (S3/MinIO) |
| social-service | 8083 | postgres-social (5435) | Follow / Unfollow |

## Arrancar localmente

```bash
# 1. Clonar y entrar al repo
git clone ...
cd feed-backend

# 2. Levantar todo (Postgres x4 + NATS + MinIO + migrate x4 + servicios)
make up

# 3. Ver logs por servicio
make logs-auth
make logs-feed
make logs-content
make logs-social

# 4. Health check de todos los servicios
make health
```

## Endpoints

### auth-service `localhost:8080`

| Método | Ruta | Auth | Descripción |
|--------|------|------|-------------|
| GET | `/health` | — | Liveness check |
| GET | `/ready` | — | Readiness check |
| POST | `/auth/register` | — | Registrar nuevo usuario |
| POST | `/auth/login` | — | Login → access + refresh token |
| POST | `/auth/refresh` | — | Renovar access token |
| POST | `/auth/logout` | JWT | Invalidar sesión |

### feed-service `localhost:8081`

| Método | Ruta | Auth | Descripción |
|--------|------|------|-------------|
| GET | `/health` | — | Liveness check |
| GET | `/ready` | — | Readiness check |
| GET | `/feed` | X-User-ID header | Feed paginado del usuario |

### content-service `localhost:8082`

| Método | Ruta | Auth | Descripción |
|--------|------|------|-------------|
| GET | `/health` | — | Liveness check |
| GET | `/ready` | — | Readiness check |
| GET | `/posts/{id}` | — | Obtener post por ID |
| POST | `/posts` | JWT | Crear post (con media en S3) |
| DELETE | `/posts/{id}` | JWT | Eliminar post |

### social-service `localhost:8083`

| Método | Ruta | Auth | Descripción |
|--------|------|------|-------------|
| GET | `/health` | — | Liveness check |
| GET | `/ready` | — | Readiness check |
| GET | `/users/{user_id}/followers` | — | Listar seguidores |
| GET | `/users/{user_id}/following` | — | Listar seguidos |
| GET | `/users/{user_id}/stats` | — | Contadores follow |
| POST | `/follow/{target_id}` | JWT | Seguir usuario |
| DELETE | `/follow/{target_id}` | JWT | Dejar de seguir |

## Eventos NATS (JetStream)

Convención de tópicos: `{dominio}.{entidad}.{acción}.{versión}`

| Tópico | Producer | Consumers |
|--------|----------|-----------|
| `auth.user.registered.v1` | auth-service | feed-service |
| `auth.user.logged_in.v1` | auth-service | analytics (fase 2) |
| `posts.created.v1` | content-service | feed-service, notification-service |
| `posts.deleted.v1` | content-service | feed-service |
| `users.followed.v1` | social-service | feed-service, notification-service |
| `users.unfollowed.v1` | social-service | feed-service |

Los mensajes fallidos van a `dlq.<tópico>` (dead letter queue).

## Estructura del monorepo

```
feed-backend/
├── shared/                   # Paquetes compartidos (events, envelope)
├── auth-service/
│   ├── cmd/server/
│   ├── internal/
│   │   ├── domain/user/      # Agregado User + eventos
│   │   ├── domain/token/     # Entidad Token + JWT service
│   │   ├── application/      # Casos de uso + puertos
│   │   └── infrastructure/   # Postgres, NATS, HTTP handler
│   └── migrations/
├── feed-service/
│   ├── cmd/server/
│   ├── internal/
│   │   ├── domain/feed/      # FeedEntry + repository port
│   │   └── infrastructure/   # Postgres, NATS consumer, HTTP handler
│   └── migrations/
├── content-service/
│   ├── cmd/server/
│   ├── internal/
│   │   ├── domain/post/      # Agregado Post + eventos
│   │   ├── domain/media/     # Entidad Media
│   │   └── infrastructure/   # Postgres, NATS, S3/MinIO, HTTP handler
│   └── migrations/
├── social-service/
│   ├── cmd/server/
│   ├── internal/
│   │   ├── domain/follow/    # Agregado Follow + eventos
│   │   └── infrastructure/   # Postgres, NATS, HTTP handler
│   └── migrations/
├── api/openapi/specs.yaml    # Especificación OpenAPI
├── docker-compose.yml
└── Makefile
```

## Regla hexagonal

```
HTTP handler → input port (UseCase)
                    │
             use case impl
             ├── domain.Repository (port) ← postgres adapter
             ├── storage.Service (port)   ← S3/MinIO adapter
             └── events.Publisher (port)  ← NATS JetStream adapter
```

**Nada del dominio o la aplicación importa paquetes de infraestructura.**

## Variables de entorno relevantes

| Variable | Servicio | Descripción |
|----------|----------|-------------|
| `JWT_SECRET` | auth, content, social | Secreto HMAC compartido para firmar/validar tokens |
| `JWT_ACCESS_TTL` | auth | TTL del access token (default `15m`) |
| `JWT_REFRESH_TTL` | auth | TTL del refresh token (default `168h`) |
| `S3_BUCKET` | content | Bucket de MinIO para media |
| `S3_ENDPOINT` | content | URL de MinIO (default `http://minio:9000`) |
| `NATS_URL` | todos | URL de NATS (default `nats://nats:4222`) |

## Comandos útiles

```bash
make build        # Compilar todos los módulos
make test         # Tests en todos los módulos (con -race)
make tidy         # go mod tidy en todos los módulos
make health       # curl /health en los 4 servicios
make nats-info    # Estado de NATS JetStream
make down         # Bajar todo y borrar volúmenes
```

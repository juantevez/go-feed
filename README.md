# My IG FEED

Servicio responsable de generar y servir el timeline personalizado de cada usuario, implementando el patrón Fan-Out-on-Write con Redis Sorted Sets y PostgreSQL para metadatos.

### Tabla de Contenidos
🎯 Responsabilidades
🏗️ Arquitectura
🚀 Inicio Rápido
⚙️ Configuración
📡 API Endpoints
🔄 Flujo Fan-Out
🧪 Testing
🐳 Docker
📊 Métricas y Observabilidad
🔧 Desarrollo
🤝 Contribuir

### Responsabilidades

| Función | Descripción |
| :--- | :--- |
| `GET /feed` | Servir timeline paginado con cursor-based pagination | × |
| `Fan-Out Worker` | Escuchar posts.created.v1 vía NATS y distribuir a seguidores | × |
| `Cache de Timeline` | Redis Sorted Sets para acceso O(log N) a posts ordenados | × |
| `Enriquecimiento` | Unir metadatos de PostgreSQL (autor, likes, comentarios) | × |
| `Filtrado` | Respetar visibilidad (public, followers_only) y soft-deletes | × |


### Arquitectura
```
┌─────────────────────────────────────────────────┐
│                 HTTP API (Chi)                  │
│  GET /feed?cursor=xxx&limit=20                  │
└─────────────────┬───────────────────────────────┘
                  │
┌─────────────────▼───────────────────────────────┐
│              FeedUseCase                        │
│  • GetFeed(): orquestar Redis + PostgreSQL      │
│  • FanOutWorker: consumir NATS → Redis          │
└─────────────────┬───────────────────────────────┘
                  │
    ┌─────────────┴─────────────────┐
    │                               │
┌───▼───────────┐           ┌────────▼────────┐
│ Redis         │           │   PostgreSQL    │
│ • ZADD        │           │ • posts         │
│ • ZREVRANGE   │           │ • follows       │
│ • Sorted Sets │           │ • users         │
└───────────────┘           └─────────────────┘
```



Capas del Código
```
services/feed/
├── cmd/server/
│   ├── main.go              # Entry point + wiring de dependencias
│   └── Dockerfile           # Build multi-stage optimizado
├── internal/
│   ├── domain/
│   │   └── feed.go          # Interfaces y tipos de dominio
│   ├── repository/
│   │   ├── redis.go         # Implementación Redis (timeline)
│   │   └── postgres.go      # Implementación PostgreSQL (metadatos)
│   ├── usecase/
│   │   ├── feed.go          # Lógica de negocio: GetFeed
│   │   └── fanout.go        # Worker NATS: posts.created.v1 → Redis
│   └── http/
│       └── handler.go       # Handler HTTP + router Chi
├── go.mod                   # Módulo: github.com/juantevez/feed-backend/services/feed
└── README.md                # Este archivo
```

### Inicio Rápido
Requisitos Previos
Go 1.25+
Docker + Docker Compose (para dependencias)
PostgreSQL 16+ corriendo con el esquema inicial aplicado
Redis 7+ corriendo
NATS 2.10+ con JetStream habilitado

Ejecutar Localmente (Desarrollo)

### 1. Levantar dependencias infraestructura
```
cd ~/go-code/feed-backend
docker compose up -d postgres redis nats
```

### 2. Esperar healthchecks (~10s)
```
docker compose ps
```

### 3. Ejecutar servicio feed
```
cd services/feed/cmd/server
export REDIS_URL="localhost:6379"
export DATABASE_URL="postgres://feed_dev:dev_password@localhost:5432/feed_db?sslmode=disable"
export FEED_PORT=":8082"

go run .
```

### 4. Probar endpoint
```
curl -H "X-User-ID: <user_uuid>" http://localhost:8082/feed
```
Respuesta de Ejemplo

```
{
  "entries": [
    {
      "post_id": "c3d4e5f6-a7b8-9012-cdef-123456789012",
      "author_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
      "username": "test_author",
      "caption": "Hello from Postgres!",
      "media_urls": ["https://img.local/1.jpg"],
      "likes": 5,
      "comments": 2,
      "created_at": "2026-05-27T10:30:00Z",
      "cursor": "2026-05-27T10:30:00Z_c3d4e5f6-a7b8-9012-cdef-123456789012"
    }
  ],
  "next_cursor": "2026-05-27T09:00:00Z_b2c3d4e5-f6a7-8901-bcde-f12345678901"
}
```
## Configuración
### Variables de Entorno

| Variable | Descripción | Default | Requerida |
| :--- | :--- | :--- | :---: |
| `REDIS_URL` | Conexión Redis (`host:port`) | `localhost:6379` | × |
| `REDIS_PASSWORD` | Password Redis (si aplica) | `""` | × |
| `REDIS_DB` | Índice de base de datos Redis | `0` | × |
| `DATABASE_URL` | DSN PostgreSQL completo | `postgres://feed_dev:dev_password@localhost:5432/feed_db?sslmode=disable` | × |
| `FEED_PORT` | Puerto para escuchar HTTP | `:8082` | × |
| `NATS_URL` | Conexión NATS para fan-out | `nats://localhost:4222` | × |
| `LOG_LEVEL` | Nivel de logging (`debug`, `info`, `warn`, `error`) | `info` | × |



### Ejemplo `.env` para Desarrollo

```
# services/feed/.env (opcional, para go run --env-file)
REDIS_URL=localhost:6379
DATABASE_URL=postgres://feed_dev:dev_password@localhost:5432/feed_db?sslmode=disable
FEED_PORT=:8082
NATS_URL=nats://localhost:4222
LOG_LEVEL=debug
```
# Instagram-style backend (Go + Hexagonal)

Backend de red social con modelo Pub/Sub, implementado en **Go 1.25** con arquitectura hexagonal y DDD.

## Stack

| Capa | Tecnología |
|------|-----------|
| API | Go 1.25 + chi |
| DB | PostgreSQL 16 |
| Mensajería | NATS 2.10 JetStream |
| Migraciones | golang-migrate |
| Contenedores | Docker + Compose |

## Arrancar localmente

```bash
# 1. Clonar y entrar al repo
git clone ...
cd my-ig

# 2. Levantar todo (Postgres + NATS + migrate + auth-service)
make up

# 3. Ver logs del servicio
make logs

# 4. Health check
curl http://localhost:8080/health
```

## Estructura del auth-service

```
auth-service/
├── cmd/server/           # Entrypoint — wiring y arranque HTTP
├── internal/
│   ├── domain/
│   │   ├── user/         # Agregado User + puerto Repository
│   │   └── token/        # Entidad Token + puerto Service
│   ├── application/
│   │   ├── port/
│   │   │   ├── input/    # Driving ports (AuthUseCase)
│   │   │   └── output/   # Driven ports (EventPublisher)
│   │   └── usecase/      # Implementación de casos de uso
│   └── infrastructure/
│       ├── persistence/postgres/   # Adaptador Postgres
│       ├── messaging/nats/         # Adaptador NATS JetStream
│       └── http/
│           ├── handler/  # HTTP handlers
│           ├── middleware/
│           └── router/   # chi router
├── migrations/           # SQL (golang-migrate)
├── configs/
├── Dockerfile
└── go.mod
```

## Regla hexagonal

```
HTTP handler → input.AuthUseCase (port)
                      │
               AuthService (use case)
               ├── user.Repository (port) ← postgres.UserRepository
               ├── token.Service (port)   ← jwt adapter (TODO)
               └── output.EventPublisher  ← nats.Publisher
```

**Nada del dominio o la aplicación importa paquetes de infraestructura.**

## Próximos pasos

1. Implementar `AuthService.Register` + `Login` (hashing bcrypt, pgx queries)
2. Adaptador JWT (`token.Service`) con RS256
3. Middleware de autenticación
4. Servicio `posts` con el mismo scaffold
5. Feed Worker consumiendo `posts.created.v1` de NATS
```

# ── Build stage ────────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copiamos shared primero — el replace directive lo referencia como ../../shared
# desde el punto de vista del módulo, pero en Docker necesitamos traerlo al contexto.
COPY shared/ ./shared/
COPY services/feed-service/ ./services/feed-service/

WORKDIR /app/services/feed-service

RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    go mod download
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /feed-service ./cmd/server

# ── Runtime stage ───────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12 AS runtime

WORKDIR /app
COPY --from=builder /feed-service /app/feed-service

EXPOSE 8081

ENTRYPOINT ["/app/feed-service"]

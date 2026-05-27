# ── Build stage ────────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

WORKDIR /app

# shared debe estar disponible para el replace directive
COPY shared/ ./shared/
COPY auth-service/ ./auth-service/

WORKDIR /app/auth-service

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /auth-service ./cmd/server

# ── Runtime stage ───────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12 AS runtime

WORKDIR /app
COPY --from=builder /auth-service /app/auth-service

EXPOSE 8080

ENTRYPOINT ["/app/auth-service"]
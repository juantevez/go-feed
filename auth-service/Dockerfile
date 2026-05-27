# ── Build stage ────────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Download dependencies first (layer cache)
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build a statically linked binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /auth-service ./cmd/server

# ── Runtime stage ───────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12 AS runtime

WORKDIR /app

COPY --from=builder /auth-service /app/auth-service

EXPOSE 8080

ENTRYPOINT ["/app/auth-service"]

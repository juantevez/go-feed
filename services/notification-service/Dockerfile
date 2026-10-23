FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY shared/ ./shared/
COPY services/notification-service/ ./services/notification-service/

WORKDIR /app/services/notification-service
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    go mod download
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /notification-service ./cmd/server

FROM gcr.io/distroless/static-debian12 AS runtime
WORKDIR /app
COPY --from=builder /notification-service /app/notification-service
EXPOSE 8084
ENTRYPOINT ["/app/notification-service"]

FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY shared/ ./shared/
COPY social-service/ ./social-service/

WORKDIR /app/social-service
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    go mod download
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /social-service ./cmd/server

FROM gcr.io/distroless/static-debian12 AS runtime
WORKDIR /app
COPY --from=builder /social-service /app/social-service
EXPOSE 8083
ENTRYPOINT ["/app/social-service"]

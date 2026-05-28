FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY shared/ ./shared/
COPY notification-service/ ./notification-service/

WORKDIR /app/notification-service
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /notification-service ./cmd/server

FROM gcr.io/distroless/static-debian12 AS runtime
WORKDIR /app
COPY --from=builder /notification-service /app/notification-service
EXPOSE 8084
ENTRYPOINT ["/app/notification-service"]

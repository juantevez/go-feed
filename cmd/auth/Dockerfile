# cmd/feed/Dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /app

# 1. Workspace config
COPY go.work go.work.sum ./

# 2. Copiar TODOS los go.mod/go.sum referenciados en go.work
# pkg/
COPY pkg/eventbus/go.mod pkg/eventbus/go.sum ./pkg/eventbus/
COPY pkg/idempotency/go.mod pkg/idempotency/go.sum ./pkg/idempotency/

# cmd/ (todos)
COPY cmd/auth/go.mod cmd/auth/go.sum ./cmd/auth/
COPY cmd/eventbus-demo/go.mod cmd/eventbus-demo/go.sum ./cmd/eventbus-demo/
COPY cmd/feed/go.mod cmd/feed/go.sum ./cmd/feed/
COPY cmd/gateway/go.mod cmd/gateway/go.sum ./cmd/gateway/
COPY cmd/content/go.mod cmd/content/go.sum ./cmd/content/
COPY cmd/interactions/go.mod cmd/interactions/go.sum ./cmd/interactions/
COPY cmd/notifications/go.mod cmd/notifications/go.sum ./cmd/notifications/
COPY cmd/users/go.mod cmd/users/go.sum ./cmd/users/

# services/
COPY services/auth/go.mod services/auth/go.sum ./services/auth/
COPY services/feed/go.mod services/feed/go.sum ./services/feed/

# 3. Descargar deps (ahora todos los módulos existen)
RUN go work sync && go mod download

# 4. Copiar código fuente y compilar
COPY . .
ARG SERVICE_NAME=auth
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /bin/app ./cmd/${SERVICE_NAME}

FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /bin/app .
EXPOSE 8080
ENTRYPOINT ["./app"]

# syntax=docker/dockerfile:1
# cmd/interactions/Dockerfile

# ─────────────────────────────────────────────────────────────
# ETAPA 1: Compilación (usa 1.25 para optimizaciones/linters)
# ─────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder
WORKDIR /app

# 1. Copiar solo configuración del workspace y módulos (CAPA DE CACHE)
COPY go.work go.work.sum ./
COPY cmd/interactions/go.mod cmd/interactions/go.sum ./cmd/interactions/
COPY pkg/ ./pkg/
COPY services/ ./services/

# Sincronizar workspace y descargar deps sin compilar código
RUN go work sync && go mod download

# 2. Copiar código fuente completo
COPY . .

# 3. Compilar binario estático
ARG SERVICE_NAME
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /bin/app ./cmd/${SERVICE_NAME}

# ─────────────────────────────────────────────────────────────
# ETAPA 2: Imagen final mínima (alpine)
# ─────────────────────────────────────────────────────────────
FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=builder /bin/app .

# Puerto por defecto (cada servicio puede usar ENTRYPOINT con flags)
EXPOSE 8080

ENTRYPOINT ["./app"]
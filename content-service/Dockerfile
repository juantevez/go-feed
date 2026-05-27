FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY shared/ ./shared/
COPY content-service/ ./content-service/

WORKDIR /app/content-service

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /content-service ./cmd/server

FROM gcr.io/distroless/static-debian12 AS runtime

WORKDIR /app
COPY --from=builder /content-service /app/content-service

EXPOSE 8082

ENTRYPOINT ["/app/content-service"]

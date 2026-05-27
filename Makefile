.PHONY: up down logs build test migrate-auth migrate-feed

# ── Docker ────────────────────────────────────────────────────────────────────
up:
	@cp -n .env.example .env 2>/dev/null || true
	docker compose up --build -d

down:
	docker compose down -v

logs-auth:
	docker compose logs -f auth-service

logs-feed:
	docker compose logs -f feed-service

# ── Build ─────────────────────────────────────────────────────────────────────
build:
	cd shared       && go build ./...
	cd auth-service && go build ./...
	cd feed-service && go build ./...

# ── Test ──────────────────────────────────────────────────────────────────────
test:
	cd shared       && go test ./... -race -count=1
	cd auth-service && go test ./... -race -count=1
	cd feed-service && go test ./... -race -count=1

# ── Tidy (correr después de agregar dependencias) ─────────────────────────────
tidy:
	cd shared       && go mod tidy
	cd auth-service && go mod tidy
	cd feed-service && go mod tidy

# ── Migrations locales ────────────────────────────────────────────────────────
migrate-auth:
	migrate -path auth-service/migrations \
	        -database "postgres://ig_user:ig_pass@localhost:5432/ig_auth?sslmode=disable" up

migrate-feed:
	migrate -path feed-service/migrations \
	        -database "postgres://ig_user:ig_pass@localhost:5433/ig_feed?sslmode=disable" up

# ── NATS monitoring ───────────────────────────────────────────────────────────
nats-info:
	curl -s http://localhost:8222/varz | jq .
	
.PHONY: up down logs migrate build lint test

# ── Docker ──────────────────────────────────────────────────────────────────
up:
	@cp -n .env.example .env 2>/dev/null || true
	docker compose up --build -d

down:
	docker compose down -v

logs:
	docker compose logs -f auth-service

restart-auth:
	docker compose restart auth-service

# ── Migrations (local, requires migrate CLI) ─────────────────────────────────
migrate-up:
	migrate -path auth-service/migrations \
	        -database "postgres://ig_user:ig_pass@localhost:5432/ig_auth?sslmode=disable" up

migrate-down:
	migrate -path auth-service/migrations \
	        -database "postgres://ig_user:ig_pass@localhost:5432/ig_auth?sslmode=disable" down 1

# ── Build ─────────────────────────────────────────────────────────────────────
build:
	cd auth-service && go build ./...

# ── Code quality ──────────────────────────────────────────────────────────────
lint:
	cd auth-service && golangci-lint run ./...

test:
	cd auth-service && go test ./... -race -count=1

# ── NATS monitoring (requires curl) ──────────────────────────────────────────
nats-info:
	curl -s http://localhost:8222/varz | jq .
	
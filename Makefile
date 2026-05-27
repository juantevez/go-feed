.PHONY: up down logs build test tidy

# ── Docker ────────────────────────────────────────────────────────────────────
up:
	@cp -n  .env 2>/dev/null || true
	docker compose up --build -d

down:
	docker compose down -v

ps:
	docker compose ps

logs-auth:
	docker compose logs -f auth-service

logs-feed:
	docker compose logs -f feed-service

logs-content:
	docker compose logs -f content-service

logs-social:
	docker compose logs -f social-service

# ── Build ─────────────────────────────────────────────────────────────────────
build:
	cd shared          && go build ./...
	cd auth-service    && go build ./...
	cd feed-service    && go build ./...
	cd content-service && go build ./...
	cd social-service  && go build ./...

# ── Test ──────────────────────────────────────────────────────────────────────
test:
	cd shared          && go test ./... -race -count=1
	cd auth-service    && go test ./... -race -count=1
	cd feed-service    && go test ./... -race -count=1
	cd content-service && go test ./... -race -count=1
	cd social-service  && go test ./... -race -count=1

# ── Tidy ──────────────────────────────────────────────────────────────────────
tidy:
	cd shared          && go mod tidy
	cd auth-service    && go mod tidy
	cd feed-service    && go mod tidy
	cd content-service && go mod tidy
	cd social-service  && go mod tidy

# ── Health checks ─────────────────────────────────────────────────────────────
health:
	@echo "── auth ──";    curl -s http://localhost:8080/health
	@echo "\n── feed ──";  curl -s http://localhost:8081/health
	@echo "\n── content ──"; curl -s http://localhost:8082/health
	@echo "\n── social ──"; curl -s http://localhost:8083/health

# ── NATS monitoring ───────────────────────────────────────────────────────────
nats-info:
	curl -s http://localhost:8222/varz | jq .

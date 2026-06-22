.PHONY: up down ps logs build test tidy health nats-streams nats-streams-ls

NATS_SERVER := nats://localhost:4222

# ── Docker ────────────────────────────────────────────────────────────────────
up:
	@cp -n .env.example .env 2>/dev/null || true
	docker compose up --build -d
	@echo "── esperando servicios (15s) ──"
	@sleep 15
	$(MAKE) nats-streams
	@echo "── listo! ──"
	$(MAKE) health

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
	@echo "── auth ──";    curl -s http://localhost:8080/health && echo ""
	@echo "── feed ──";    curl -s http://localhost:8081/health && echo ""
	@echo "── content ──"; curl -s http://localhost:8082/health && echo ""
	@echo "── social ──";  curl -s http://localhost:8083/health && echo ""

# ── NATS streams ──────────────────────────────────────────────────────────────
# Usa la API HTTP de NATS JetStream directamente — sin CLI ni confirmaciones.
# Idempotente: PUT crea o actualiza sin errores si ya existe.
nats-streams:
	@echo "── NATS: creando streams via API ──"
	@curl -s -X PUT http://localhost:8222/v1/stream \
	  -H "Content-Type: application/json" \
	  -d '{"name":"posts","subjects":["posts.created.v1","posts.deleted.v1"],"storage":"file","retention":"limits","max_age":86400000000000}' \
	  > /dev/null || true
	@curl -s -X PUT http://localhost:8222/v1/stream \
	  -H "Content-Type: application/json" \
	  -d '{"name":"auth","subjects":["auth.user.registered.v1","auth.user.logged_in.v1"],"storage":"file","retention":"limits","max_age":86400000000000}' \
	  > /dev/null || true
	@curl -s -X PUT http://localhost:8222/v1/stream \
	  -H "Content-Type: application/json" \
	  -d '{"name":"social","subjects":["users.followed.v1","users.unfollowed.v1"],"storage":"file","retention":"limits","max_age":86400000000000}' \
	  > /dev/null || true
	@echo "── NATS: streams listos ──"

nats-streams-ls:
	@nats stream ls --server $(NATS_SERVER)

# ── NATS monitoring ───────────────────────────────────────────────────────────
nats-info:
	curl -s http://localhost:8222/varz | jq .

logs-notification:
	docker compose logs -f notification-service
	
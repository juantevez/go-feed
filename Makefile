.PHONY: up down ps build test tidy health nats-streams-ls dlq-list dlq-show dlq-replay dlq-replay-all dlq-discard

NATS_SERVER := nats://localhost:4222

# Monorepo: servicios en services/, compose en deploy/docker/. El .env de la raíz
# define COMPOSE_FILE, así `docker compose` funciona desde acá sin -f (no usar -f:
# con -f Compose busca el .env junto al compose y no el de la raíz).
COMPOSE  := docker compose
SERVICES := auth-service content-service feed-service notification-service social-service
MODULES  := shared $(addprefix services/,$(SERVICES))
# go.work: desde la raíz hay que nombrar cada módulo (./... no cruza módulos).
PKGS     := $(addsuffix /...,$(addprefix ./,$(MODULES)))

# ── Docker ────────────────────────────────────────────────────────────────────
.env:
	cp .env.example .env

up: .env
	$(COMPOSE) up --build -d
	@echo "── esperando servicios (15s) ──"
	@sleep 15
	@echo "── listo! ──"
	$(MAKE) health

down: .env
	$(COMPOSE) down -v

ps: .env
	$(COMPOSE) ps

logs-%: .env            # make logs-auth | logs-feed | logs-content | logs-social | logs-notification
	$(COMPOSE) logs -f $*-service

# ── Build ─────────────────────────────────────────────────────────────────────
build:
	go build $(PKGS)

# ── Test ──────────────────────────────────────────────────────────────────────
test:
	go test $(PKGS) -race -count=1

# ── Tidy ──────────────────────────────────────────────────────────────────────
# go mod tidy es por módulo (no usa go.work), y después sincronizamos el workspace.
tidy:
	@for m in $(MODULES); do echo "── $$m"; (cd $$m && go mod tidy) || exit 1; done
	go work sync

# ── Health checks ─────────────────────────────────────────────────────────────
health:
	@echo "── auth ──";    curl -s http://localhost:8080/health && echo ""
	@echo "── feed ──";    curl -s http://localhost:8081/health && echo ""
	@echo "── content ──"; curl -s http://localhost:8082/health && echo ""
	@echo "── social ──";  curl -s http://localhost:8083/health && echo ""

# ── NATS streams ──────────────────────────────────────────────────────────────
# Los crea cada servicio al arrancar (shared/streams, más DLQ y el KV de tokens
# revocados): no hace falta crearlos a mano.
nats-streams-ls:
	@nats stream ls --server $(NATS_SERVER)

# ── Dead letter queue ─────────────────────────────────────────────────────────
# Mensajes que fallaron 3 veces (o con payload inválido). Ver shared/dlq.
DLQ := cd shared && go run ./cmd/dlq -server $(NATS_SERVER)

dlq-list:
	@$(DLQ) list

dlq-show:          # make dlq-show SEQ=12
	@$(DLQ) show $(SEQ)

dlq-replay:        # make dlq-replay SEQ=12
	@$(DLQ) replay $(SEQ)

dlq-replay-all:    # make dlq-replay-all [CONSUMER=feed-service-post-created]
	@$(DLQ) -consumer "$(CONSUMER)" replay all

dlq-discard:       # make dlq-discard SEQ=12
	@$(DLQ) discard $(SEQ)

# ── NATS monitoring ───────────────────────────────────────────────────────────
nats-info:
	curl -s http://localhost:8222/varz | jq .
	
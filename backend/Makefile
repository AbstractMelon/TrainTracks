# Local development for backend/. Run `make` for a list of targets.

COMPOSE := docker compose -f ../deploy/docker-compose.dev.yml

.PHONY: help setup env infra down migrate seed dev api ingestor test test-integration clean

help: ## Show available targets
	@grep -E '^[a-z_-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-16s %s\n", $$1, $$2}'

setup: migrate ## Bootstrap dev: .env, Postgres+Redis, schema migrations

env: ## Create .env from .env.example when missing
	@test -f .env || cp .env.example .env

infra: ## Start dev Postgres (15432) and Redis (16379)
	$(COMPOSE) up -d

down: ## Stop dev infra
	$(COMPOSE) down

migrate: env infra ## Apply SQL migrations (starts dev infra)
	go run ./cmd/migrate up

seed: env ## Download and load GTFS static data (routes, stations, trips)
	go run ./cmd/gtfs-import

dev: ## Run API + ingestor together (Ctrl+C stops both)
	@$(MAKE) --no-print-directory -j2 api ingestor

api: env ## Run only the API under air
	@mkdir -p tmp
	air -c .air.toml

ingestor: env ## Run only the ingestor under air
	@mkdir -p tmp
	air -c .air.ingestor.toml

test: ## Unit tests
	go test ./internal/...

test-integration: ## Integration tests (needs Postgres+Redis running)
	TRAINTRACKS_INTEGRATION=1 go test ./internal/integration/...

clean: ## Remove air build output
	rm -rf tmp

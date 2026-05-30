.PHONY: help tidy build web-build run test test-core test-race lint migrate-up migrate-down seed real-rag-reindex demo demo-web demo-web-real demo-pg demo-pg-full demo-mock demo-real demo-real-full load-test docker-up docker-up-cluster docker-down clean

GO ?= go
APP := bin/server
CONFIG ?= config/config.yaml.example

help: ## show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'

tidy: ## go mod tidy
	$(GO) mod tidy

build: ## build server binary
	$(GO) build -o $(APP) ./cmd/server

web-build: ## build React web frontend
	npm --prefix web run build

run: build ## run server with local config
	$(APP) -config $(CONFIG)

test: ## run unit tests
	$(GO) test ./... -count=1

test-core: ## run core regression packages for web demo and config
	$(GO) test ./internal/parser ./internal/httpapi ./cmd/server ./internal/config -count=1

test-race: ## run unit tests with race detector
	$(GO) test ./... -race -count=1

lint: ## run golangci-lint
	golangci-lint run ./...

migrate-up: ## apply DB migrations (requires docker postgres up)
	psql "$$INTERVIEW_POSTGRES_DSN" -v ON_ERROR_STOP=1 -f migrations/001_init.up.sql
	psql "$$INTERVIEW_POSTGRES_DSN" -v ON_ERROR_STOP=1 -f migrations/002_question_bank_expected_points.up.sql
	psql "$$INTERVIEW_POSTGRES_DSN" -v ON_ERROR_STOP=1 -f migrations/003_question_bank_metadata.up.sql
	psql "$$INTERVIEW_POSTGRES_DSN" -v ON_ERROR_STOP=1 -f migrations/004_question_bank_imports.up.sql
	psql "$$INTERVIEW_POSTGRES_DSN" -v ON_ERROR_STOP=1 -f migrations/005_question_bank_embedding_status.up.sql
	psql "$$INTERVIEW_POSTGRES_DSN" -v ON_ERROR_STOP=1 -f migrations/006_question_bank_import_lease.up.sql
	psql "$$INTERVIEW_POSTGRES_DSN" -v ON_ERROR_STOP=1 -f migrations/007_question_bank_import_review_status.up.sql
	psql "$$INTERVIEW_POSTGRES_DSN" -v ON_ERROR_STOP=1 -f migrations/008_question_bank_import_field_provenance.up.sql

migrate-down: ## roll back DB migrations
	psql "$$INTERVIEW_POSTGRES_DSN" -v ON_ERROR_STOP=1 -f migrations/008_question_bank_import_field_provenance.down.sql
	psql "$$INTERVIEW_POSTGRES_DSN" -v ON_ERROR_STOP=1 -f migrations/007_question_bank_import_review_status.down.sql
	psql "$$INTERVIEW_POSTGRES_DSN" -v ON_ERROR_STOP=1 -f migrations/006_question_bank_import_lease.down.sql
	psql "$$INTERVIEW_POSTGRES_DSN" -v ON_ERROR_STOP=1 -f migrations/005_question_bank_embedding_status.down.sql
	psql "$$INTERVIEW_POSTGRES_DSN" -v ON_ERROR_STOP=1 -f migrations/004_question_bank_imports.down.sql
	psql "$$INTERVIEW_POSTGRES_DSN" -v ON_ERROR_STOP=1 -f migrations/003_question_bank_metadata.down.sql
	psql "$$INTERVIEW_POSTGRES_DSN" -v ON_ERROR_STOP=1 -f migrations/002_question_bank_expected_points.down.sql
	psql "$$INTERVIEW_POSTGRES_DSN" -v ON_ERROR_STOP=1 -f migrations/001_init.down.sql

seed: ## load demo question bank rows
	psql "$$INTERVIEW_POSTGRES_DSN" -v ON_ERROR_STOP=1 -f migrations/seed_question_bank.sql

real-rag-reindex: ## rebuild question_bank embeddings with real embedding API
	@[ -n "$$INTERVIEW_EMBEDDING_API_KEY" ] || (echo "INTERVIEW_EMBEDDING_API_KEY required" && exit 1)
	$(GO) run ./cmd/reindex -seed seeds/question_bank.json -mode real -base-url "$${INTERVIEW_EMBEDDING_BASE_URL:-http://127.0.0.1:8000/v1}" -model "$${INTERVIEW_EMBEDDING_MODEL:-BAAI/bge-m3}" -dim "$${INTERVIEW_EMBEDDING_DIMENSION:-1024}"

demo: build ## smoke test: start, ping, stop
	sh ./scripts/smoke.sh

demo-web: web-build ## run web interview server in mock mode
	INTERVIEW_LLM_MODE=mock INTERVIEW_EMBEDDING_MODE=mock $(GO) run ./cmd/server -config $(CONFIG)

demo-web-real: web-build ## run web interview server with real LLM; API key may come from env or YAML
	INTERVIEW_LLM_MODE=real INTERVIEW_EMBEDDING_MODE=mock $(GO) run ./cmd/server -config $(CONFIG)

demo-pg: build ## smoke test against configured PG DSN
	sh ./scripts/smoke.sh

demo-pg-full: docker-up build ## migrate, seed, and smoke test against configured PG DSN
	$(MAKE) migrate-up
	$(MAKE) seed
	sh ./scripts/smoke.sh

demo-mock: ## run cmd/demo end-to-end against mock LLM
	INTERVIEW_LLM_MODE=mock INTERVIEW_EMBEDDING_MODE=mock \
	$(GO) run ./cmd/demo -config config/config.yaml.example -script testdata/demo/example.yaml

demo-real: ## run cmd/demo end-to-end against real LLM (requires INTERVIEW_LLM_API_KEY)
	@[ -n "$$INTERVIEW_LLM_API_KEY" ] || (echo "INTERVIEW_LLM_API_KEY required" && exit 1)
	INTERVIEW_LLM_MODE=real INTERVIEW_EMBEDDING_MODE=mock \
	$(GO) run ./cmd/demo -config config/config.yaml.example -script testdata/demo/example.yaml

demo-real-full: ## run Docker PG/Redis + real embedding reindex + real CLI/Web E2E
	pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/real_e2e.ps1

load-test: ## run k6 load test against local cluster (stage 8)
	docker run --rm -i --network=host grafana/k6 run - < chaos/k6_load_1000users.js

docker-up: ## start single-instance docker stack (pg + redis)
	docker compose up -d postgres redis

docker-up-cluster: ## start 3-instance cluster + nginx LB
	docker compose --profile cluster up -d --build

docker-down: ## stop everything
	docker compose down -v

clean: ## remove build artifacts
	rm -rf bin/

.PHONY: help tidy build run test test-race lint migrate-up migrate-down seed demo demo-pg demo-pg-full demo-mock demo-real load-test docker-up docker-up-cluster docker-down clean

GO ?= go
APP := bin/server

help: ## show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'

tidy: ## go mod tidy
	$(GO) mod tidy

build: ## build server binary
	$(GO) build -o $(APP) ./cmd/server

run: build ## run server with local config
	$(APP) -config config/config.yaml.example

test: ## run unit tests
	$(GO) test ./... -count=1

test-race: ## run unit tests with race detector
	$(GO) test ./... -race -count=1

lint: ## run golangci-lint
	golangci-lint run ./...

migrate-up: ## apply DB migrations (requires docker postgres up)
	psql "$$INTERVIEW_POSTGRES_DSN" -v ON_ERROR_STOP=1 -f migrations/001_init.up.sql

migrate-down: ## roll back DB migrations
	psql "$$INTERVIEW_POSTGRES_DSN" -v ON_ERROR_STOP=1 -f migrations/001_init.down.sql

seed: ## load demo question bank rows
	psql "$$INTERVIEW_POSTGRES_DSN" -v ON_ERROR_STOP=1 -f migrations/seed_question_bank.sql

demo: build ## smoke test: start, ping, stop
	sh ./scripts/smoke.sh

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

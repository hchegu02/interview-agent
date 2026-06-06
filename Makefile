.PHONY: help tidy build web-build run test test-core test-race lint verify-local verify-agent eval-rag questionbank-lint questionbank-lint-strict eval-mock migrate-up migrate-down seed real-rag-reindex demo demo-web demo-web-real demo-pg demo-pg-full demo-mock demo-real demo-real-full e2e-smoke load-test chaos-dry-run docker-up docker-up-cluster docker-down clean

GO ?= go
APP := bin/server
CONFIG ?= config/config.yaml.example
PWSH ?= pwsh -NoProfile -ExecutionPolicy Bypass

help: ## show this help
	@$(PWSH) -Command 'Get-Content "Makefile" | ForEach-Object { if ($$_ -match "^([a-zA-Z_-]+):.*?## (.*)$$") { "  {0,-18} {1}" -f $$matches[1], $$matches[2] } }'

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

verify-local: ## run dependency-free local quality gate
	$(GO) test ./... -count=1
	npm --prefix web run test
	npm --prefix web run build
	$(MAKE) verify-agent
	$(MAKE) eval-rag
	$(MAKE) questionbank-lint
	$(MAKE) eval-mock
	git diff --check

verify-agent: ## run Agent output verification fixture
	$(GO) run ./cmd/agent-verify -session testdata/agent_verify/pass_session.json

eval-rag: ## run offline RAG retrieval evaluation
	$(GO) run ./cmd/rag-eval -cases testdata/rag/golden_queries.jsonl -config $(CONFIG) -out tmp/eval/rag -min-recall-at-5 0.70 -min-recall-at-10 0.80 -min-mrr-at-k 0.90 -min-ndcg-at-k 0.75 -min-group-cases 3 -min-group-recall-at-5 0.50 -min-stage-recall-at-5 vector=0.70,bm25=0.65,rule=0.60,rrf=0.75,rerank=0.70 -min-stage-mrr-at-k rrf=0.88,rerank=0.90

questionbank-lint: ## lint seed question bank metadata quality
	$(GO) run ./cmd/questionbank-lint -seed seeds/question_bank.json -min-expected-points 3 -min-scenario-ratio 0.05

questionbank-lint-strict: ## enforce target seed metadata quality gate
	$(GO) run ./cmd/questionbank-lint -seed seeds/question_bank.json -min-expected-points 3 -min-scenario-ratio 0.8

eval-mock: ## run offline mock evaluation fixtures
	$(GO) run ./cmd/eval -suite testdata/eval -mode mock -out tmp/eval/mock

migrate-up: ## apply DB migrations (requires docker postgres up)
	$(PWSH) -Command 'psql $$env:INTERVIEW_POSTGRES_DSN -v ON_ERROR_STOP=1 -f migrations/001_init.up.sql'
	$(PWSH) -Command 'psql $$env:INTERVIEW_POSTGRES_DSN -v ON_ERROR_STOP=1 -f migrations/002_question_bank_expected_points.up.sql'
	$(PWSH) -Command 'psql $$env:INTERVIEW_POSTGRES_DSN -v ON_ERROR_STOP=1 -f migrations/003_question_bank_metadata.up.sql'
	$(PWSH) -Command 'psql $$env:INTERVIEW_POSTGRES_DSN -v ON_ERROR_STOP=1 -f migrations/004_question_bank_imports.up.sql'
	$(PWSH) -Command 'psql $$env:INTERVIEW_POSTGRES_DSN -v ON_ERROR_STOP=1 -f migrations/005_question_bank_embedding_status.up.sql'
	$(PWSH) -Command 'psql $$env:INTERVIEW_POSTGRES_DSN -v ON_ERROR_STOP=1 -f migrations/006_question_bank_import_lease.up.sql'
	$(PWSH) -Command 'psql $$env:INTERVIEW_POSTGRES_DSN -v ON_ERROR_STOP=1 -f migrations/007_question_bank_import_review_status.up.sql'
	$(PWSH) -Command 'psql $$env:INTERVIEW_POSTGRES_DSN -v ON_ERROR_STOP=1 -f migrations/008_question_bank_import_field_provenance.up.sql'
	$(PWSH) -Command 'psql $$env:INTERVIEW_POSTGRES_DSN -v ON_ERROR_STOP=1 -f migrations/009_question_bank_content_trgm.up.sql'
	$(PWSH) -Command 'psql $$env:INTERVIEW_POSTGRES_DSN -v ON_ERROR_STOP=1 -f migrations/010_session_row_version.up.sql'

migrate-down: ## roll back DB migrations
	$(PWSH) -Command 'psql $$env:INTERVIEW_POSTGRES_DSN -v ON_ERROR_STOP=1 -f migrations/010_session_row_version.down.sql'
	$(PWSH) -Command 'psql $$env:INTERVIEW_POSTGRES_DSN -v ON_ERROR_STOP=1 -f migrations/009_question_bank_content_trgm.down.sql'
	$(PWSH) -Command 'psql $$env:INTERVIEW_POSTGRES_DSN -v ON_ERROR_STOP=1 -f migrations/008_question_bank_import_field_provenance.down.sql'
	$(PWSH) -Command 'psql $$env:INTERVIEW_POSTGRES_DSN -v ON_ERROR_STOP=1 -f migrations/007_question_bank_import_review_status.down.sql'
	$(PWSH) -Command 'psql $$env:INTERVIEW_POSTGRES_DSN -v ON_ERROR_STOP=1 -f migrations/006_question_bank_import_lease.down.sql'
	$(PWSH) -Command 'psql $$env:INTERVIEW_POSTGRES_DSN -v ON_ERROR_STOP=1 -f migrations/005_question_bank_embedding_status.down.sql'
	$(PWSH) -Command 'psql $$env:INTERVIEW_POSTGRES_DSN -v ON_ERROR_STOP=1 -f migrations/004_question_bank_imports.down.sql'
	$(PWSH) -Command 'psql $$env:INTERVIEW_POSTGRES_DSN -v ON_ERROR_STOP=1 -f migrations/003_question_bank_metadata.down.sql'
	$(PWSH) -Command 'psql $$env:INTERVIEW_POSTGRES_DSN -v ON_ERROR_STOP=1 -f migrations/002_question_bank_expected_points.down.sql'
	$(PWSH) -Command 'psql $$env:INTERVIEW_POSTGRES_DSN -v ON_ERROR_STOP=1 -f migrations/001_init.down.sql'

seed: ## load demo question bank rows
	$(PWSH) -Command 'psql $$env:INTERVIEW_POSTGRES_DSN -v ON_ERROR_STOP=1 -f migrations/seed_question_bank.sql'

real-rag-reindex: ## rebuild question_bank embeddings with real embedding API
	@$(PWSH) -Command 'if (-not $$env:INTERVIEW_EMBEDDING_API_KEY) { throw "INTERVIEW_EMBEDDING_API_KEY required" }; $$baseUrl = if ($$env:INTERVIEW_EMBEDDING_BASE_URL) { $$env:INTERVIEW_EMBEDDING_BASE_URL } else { "http://127.0.0.1:8000/v1" }; $$model = if ($$env:INTERVIEW_EMBEDDING_MODEL) { $$env:INTERVIEW_EMBEDDING_MODEL } else { "BAAI/bge-m3" }; $$dim = if ($$env:INTERVIEW_EMBEDDING_DIMENSION) { $$env:INTERVIEW_EMBEDDING_DIMENSION } else { "1024" }; $(GO) run ./cmd/reindex -seed seeds/question_bank.json -mode real -base-url $$baseUrl -model $$model -dim $$dim'

demo: build ## smoke test: start, ping, stop
	$(PWSH) -File scripts/smoke.ps1 -ServerBin ".\$(APP)" -ConfigPath "$(CONFIG)"

demo-web: web-build ## run web interview server in mock mode
	$(PWSH) -Command '$$env:INTERVIEW_LLM_MODE="mock"; $$env:INTERVIEW_EMBEDDING_MODE="mock"; $(GO) run ./cmd/server -config "$(CONFIG)"'

demo-web-real: web-build ## run web interview server with real LLM; API key may come from env or YAML
	$(PWSH) -Command '$$env:INTERVIEW_LLM_MODE="real"; $$env:INTERVIEW_EMBEDDING_MODE="mock"; $(GO) run ./cmd/server -config "$(CONFIG)"'

demo-pg: build ## smoke test against configured PG DSN
	$(PWSH) -File scripts/smoke.ps1 -ServerBin ".\$(APP)" -ConfigPath "$(CONFIG)"

demo-pg-full: docker-up build ## migrate, seed, and smoke test against configured PG DSN
	$(MAKE) migrate-up
	$(MAKE) seed
	$(PWSH) -File scripts/smoke.ps1 -ServerBin ".\$(APP)" -ConfigPath "$(CONFIG)"

demo-mock: ## run cmd/demo end-to-end against mock LLM
	$(PWSH) -Command '$$env:INTERVIEW_LLM_MODE="mock"; $$env:INTERVIEW_EMBEDDING_MODE="mock"; $(GO) run ./cmd/demo -config config/config.yaml.example -script testdata/demo/example.yaml'

demo-real: ## run cmd/demo end-to-end against real LLM (requires INTERVIEW_LLM_API_KEY)
	@$(PWSH) -Command 'if (-not $$env:INTERVIEW_LLM_API_KEY) { throw "INTERVIEW_LLM_API_KEY required" }; $$env:INTERVIEW_LLM_MODE="real"; $$env:INTERVIEW_EMBEDDING_MODE="mock"; $(GO) run ./cmd/demo -config config/config.yaml.example -script testdata/demo/example.yaml'

demo-real-full: ## run Docker PG/Redis + real embedding reindex + real CLI/Web E2E
	pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/real_e2e.ps1

e2e-smoke: build ## run Windows-friendly HTTP/SSE/API e2e smoke
	pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/e2e_smoke.ps1

load-test: ## run k6 load test against local cluster
	pwsh -NoProfile -ExecutionPolicy Bypass -Command '$$d = Join-Path "tmp\chaos" (Get-Date -Format "yyyyMMdd-HHmmssfff"); New-Item -ItemType Directory -Force -Path $$d | Out-Null; $$env:K6_SUMMARY_PATH = Join-Path $$d "summary.json"; k6 run chaos/k6_load_1000users.js'

chaos-dry-run: ## validate chaos restart scripts without restarting services
	pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/chaos_redis_restart.ps1 -DryRun -SkipReadyCheck
	pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/chaos_pg_restart.ps1 -DryRun -SkipReadyCheck

docker-up: ## start single-instance docker stack (pg + redis)
	docker compose up -d postgres redis

docker-up-cluster: ## start 3-instance cluster + nginx LB
	docker compose --profile cluster up -d --build

docker-down: ## stop everything
	docker compose down -v

clean: ## remove build artifacts
	$(PWSH) -Command 'Remove-Item -LiteralPath "bin" -Recurse -Force -ErrorAction SilentlyContinue'

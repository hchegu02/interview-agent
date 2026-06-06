# AI Coding 常用命令

本文只记录 AI 开发和验证常用命令。架构说明见 `docs/SDD-Backend.md` 和 `docs/SDD-Frontend.md`。

## Go

```powershell
go test ./...
go test ./internal/httpapi -count=1
go test ./internal/nodes -count=1
go test ./internal/retriever -count=1
gofmt -w "path\to\file.go"
```

## 前端

```powershell
npm --prefix web run test
npm --prefix web run build
```

## RAG 与题库

```powershell
go run ./cmd/rag-eval -cases testdata/rag/golden_queries.jsonl -config config/config.yaml.example -out tmp/eval/rag -min-recall-at-5 0.70 -min-recall-at-10 0.80 -min-mrr-at-k 0.90 -min-ndcg-at-k 0.75 -min-group-cases 3 -min-group-recall-at-5 0.50 -min-stage-recall-at-5 vector=0.70,bm25=0.65,rule=0.60,rrf=0.75,rerank=0.70 -min-stage-mrr-at-k rrf=0.88,rerank=0.90
go run ./cmd/questionbank-lint -seed seeds/question_bank.json -min-expected-points 3 -min-scenario-ratio 0.8
go run ./cmd/reindex -config config/config.yaml.example
```

## Agent 验证

```powershell
go run ./cmd/agent-verify -session testdata/agent_verify/pass_session.json
```

## 本地质量门禁

```powershell
mingw32-make verify-agent
mingw32-make eval-rag
mingw32-make verify-local
```

## 本地服务

```powershell
go run ./cmd/server -config config/config.yaml.example
```

## Git

```powershell
git status --short --untracked-files=all
git status --short --branch
git diff --name-status
git diff --cached --name-status
git add "file1" "file2"
git commit -m "type: message"
git log --oneline -5
git log --oneline "origin/main..HEAD"
```

禁止默认使用：

```powershell
git add .
git add -A
```

## OpenSpec

```powershell
openspec list --json
openspec validate <change> --strict
openspec archive <change> --yes
openspec validate <capability> --strict
```

如果 OpenSpec CLI 在沙箱内因访问用户目录失败，需要提升权限后重跑同一条命令；不能把沙箱错误当成 spec 通过。

## 阶段收口组合

```powershell
go test ./... -count=1
mingw32-make verify-agent
openspec validate <change> --strict
openspec archive <change> --yes
openspec validate <capability> --strict
git status --short --branch
git diff --cached --name-status
```

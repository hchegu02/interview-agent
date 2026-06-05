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
go run ./cmd/rag-eval -config testdata/rag_eval/config.json -min-recall-at-5 0.6 -min-mrr 0.4 -min-ndcg-at-5 0.5
go run ./cmd/questionbank-lint -input seeds/question_bank.json
go run ./cmd/reindex -config config/config.yaml.example
```

## Agent 验证

```powershell
go run ./cmd/agent-verify -session testdata/agent_verify/pass_session.json
```

## 本地服务

```powershell
go run ./cmd/server -config config/config.yaml.example
```

## Git

```powershell
git status --short --untracked-files=all
git diff --name-status
git diff --cached --name-status
git add "file1" "file2"
git commit -m "type: message"
```

禁止默认使用：

```powershell
git add .
git add -A
```

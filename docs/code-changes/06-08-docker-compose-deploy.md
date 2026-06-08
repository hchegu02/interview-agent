# 06-08 Docker Compose 部署补齐

## 变更概述

本次补齐 Docker 本地部署入口：默认 `docker compose up -d --build` 可启动单实例 app、PostgreSQL/pgvector 和 Redis；保留三实例 cluster profile；新增可选 BGE-M3 embedding 镜像入口，但不放入默认启动链路，避免首次 compose 因模型下载和资源占用卡住。

影响范围限定在部署配置和文档，不修改 Go/React 业务代码、HTTP API、数据库 schema、Session JSON 或题库数据格式。

## 变更文件

- `docker-compose.yml`：新增默认 `app` service、可选 `bge` profile service，并把 cluster nginx 默认端口改为 `18080`，避免和默认 app 的 `8080` 冲突。
- `compose.env.example`：新增 compose 环境变量示例，使用 `COMPOSE_` 前缀区分 compose 输入和应用运行时 `INTERVIEW_*`，避免宿主调试环境污染容器配置。
- `tools/bge_server/Dockerfile`：新增 BGE-M3 OpenAI-compatible embedding 服务镜像构建文件。
- `README.md`：更新 Docker 启动说明，写明单实例、依赖、cluster、BGE profile 和 real embedding 配置。

## 函数级说明

本次没有新增或修改 Go 函数、方法、组件、Hook、API handler 或导出符号。

部署行为变化：

- `docker-compose.yml services.app`：使用根目录 `Dockerfile` 构建应用镜像，从 `COMPOSE_*` 读取 compose 输入，再注入容器内 `INTERVIEW_*` 环境变量；默认注入 PG/Redis DSN、mock LLM、mock embedding 和 redis_lua 限流后端，依赖 `postgres`、`redis` healthcheck。
- `docker-compose.yml services.bge`：使用 `tools/bge_server/Dockerfile` 构建本地 BGE-M3 embedding 服务，暴露 `8000`，挂载 `bge_cache` 保存 HuggingFace 模型缓存，提供 `/healthz` healthcheck。
- `docker-compose.yml services.nginx`：cluster profile 的 nginx 默认 host 端口从 `8080` 调整为 `18080`，避免默认 `app` service 端口冲突。

## 调用链

单实例 Docker 启动：

`docker compose up -d --build`
-> build root `Dockerfile`
-> build web assets
-> build `cmd/server`
-> start `postgres` and `redis`
-> start `app`
-> `/app/server -config /app/config/config.yaml.example`
-> `cmd/server main`
-> `config.Load`
-> HTTP routes and embedded Web assets served on `:8080`

BGE Docker 启动：

`docker compose --profile bge up -d --build bge`
-> build `tools/bge_server/Dockerfile`
-> `python -m uvicorn bge_server:app --host 0.0.0.0 --port 8000`
-> `tools/bge_server/bge_server.py`
-> `/healthz` and `/v1/embeddings`

Cluster Docker 启动：

`docker compose --profile cluster up -d --build`
-> start default services plus `app1/app2/app3/nginx`
-> nginx listens on host `18080`
-> forwards to app replicas on container port `8080`

## 数据流

默认单实例：

HTTP/SSE/Web request
-> `app`
-> PostgreSQL DSN `postgres://interview:interview@postgres:5432/interview?sslmode=disable`
-> Redis URL `redis://redis:6379/0`
-> mock LLM and mock embedding

可选 BGE：

app embedding request
-> `INTERVIEW_EMBEDDING_BASE_URL=http://bge:8000/v1`
-> `bge` service `/v1/embeddings`
-> normalized 1024 维 BGE-M3 vector
-> app embedding dimension check
-> question-bank embedding / RAG query embedding call sites

## 依赖与副作用

- 新增 Docker 构建依赖：`tools/bge_server/Dockerfile` 使用 `python:3.11-slim` 并安装 `tools/bge_server/requirements.txt`。
- 新增 Docker volume：`bge_cache`，用于 HuggingFace 模型缓存。
- `docker compose up -d --build` 会构建 app 镜像并启动 PG/Redis/app。
- `docker compose --profile bge up -d --build bge` 首次运行会下载 BGE-M3 模型，可能耗时较长并占用磁盘。
- Compose 配置输入使用 `COMPOSE_*` 前缀；容器内运行时环境仍是项目已有的 `INTERVIEW_*`，避免本机 `.env` 中的 `INTERVIEW_*` 被 Docker Compose 自动读取后改变默认部署模式。
- 不新增密钥文件，不提交 `.env`，`compose.env.example` 只包含空 key 示例。

## 测试

已执行：

- PowerShell 结构校验：通过，确认默认 `app` service、`bge` service、BGE Dockerfile、compose env 示例和 code-changes 文档存在。
- `docker compose config`：通过，默认 app 解析为 mock LLM、mock embedding，host `8080`。
- `docker compose --profile bge config`：通过，BGE profile、`8000` 端口和 `bge_cache` volume 可解析。
- `docker compose --profile cluster config`：通过，默认 app host `8080`，cluster nginx host `18080`。
- `git diff --check`：通过；输出仅包含既有/本次工作区 LF/CRLF 提示，没有 whitespace error。
- 候选提交文件密钥痕迹检查：通过；只命中 README 中的占位示例 `sk-...`，未发现真实 key。
- `docker compose build app`：通过，成功构建 `interview-agent-app:latest`。

部分验证：

- `docker compose --profile bge build bge`：执行 3 分钟后超时，未完成。超时后已清理本轮残留的 `docker/docker-compose/docker-buildx` 客户端进程。该镜像依赖 `sentence-transformers`/`torch`，构建时间和网络下载体积较大；本次只确认了 BGE compose 配置可解析，未声明 BGE 镜像构建通过。

未执行：

- 未执行 `docker compose up -d --build` 启动完整栈。
- 未执行 BGE 容器启动和 `/v1/embeddings` 维度验证。

## 风险

- Cluster profile 现在默认 nginx host 端口是 `18080`，不是旧的 `8080`；这是为避免默认 app 与 cluster nginx 端口冲突。
- BGE 容器默认 CPU 运行，首次模型下载和推理速度取决于本机资源。
- 当前 compose real LLM/embedding 仍依赖 `COMPOSE_*` 环境变量注入，不能把密钥写进仓库。

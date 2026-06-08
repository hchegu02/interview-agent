# 06-08 start local real script

## 1. 变更概述

新增正式本地启动脚本 `scripts/start-local-real.ps1`，用于一键启动内部试用需要的本地运行链路：PostgreSQL、Redis、本地 BGE-M3 embedding、Go 后端和 Vite 前端。

脚本默认读取 `D:\Desktop\env1.txt` 中的 `$env:...` 形式配置，避免在命令行暴露 LLM key。启动完成后默认打开题库管理页面。

## 2. 变更文件

- `scripts/start-local-real.ps1`
  - 新增本地真实链路启动脚本。
  - 支持 `-NoBrowser`、`-SkipDocker`、`-SkipEmbedding`、`-LLMMode real|mock`、`-EnvFile` 等参数。
  - 日志输出到 `tmp/server/*-<timestamp>.out.log` 和 `tmp/server/*-<timestamp>.err.log`。

## 3. 函数级说明

- `Write-Step`
  - 输出带时间戳的启动进度。

- `Import-EnvFile`
  - 读取 `$env:NAME=value` 格式的本地环境文件。
  - 不打印变量值，避免泄露 key。

- `Test-Url`
  - 使用 `Invoke-WebRequest` 判断本地 HTTP 地址是否可达。

- `Wait-Url`
  - 轮询等待 HTTP 地址可用；超时抛错。

- `Start-LoggedProcess`
  - 用 `Start-Process` 后台启动进程，并把 stdout/stderr 重定向到 `tmp/server`。

- `Ensure-ServerBin`
  - 如果 `tmp/server/interview-server.exe` 不存在，则执行 `go build -o ... ./cmd/server`。

- `Get-EmbeddingHealthUrl`
  - 从 embedding base URL 推导 `/healthz` 地址。

## 4. 调用链

用户从 PowerShell 执行：

```powershell
.\scripts\start-local-real.ps1
```

脚本执行：

1. 读取 env 文件。
2. 设置真实 LLM、embedding、本地 spool、PG/Redis 默认配置。
3. 可选启动 `docker compose up -d postgres redis`。
4. 可选启动 BGE-M3 uvicorn 服务。
5. 启动 Go 后端并等待 `/api/ping`。
6. 启动 Vite 前端并等待首页可达。
7. 打开 `/questions`。

## 5. 数据流

环境变量来自用户本地 `env1.txt` 或现有 shell 环境。脚本只把变量注入当前脚本进程及其子进程，不写入系统全局环境变量。

## 6. 依赖与副作用

- 依赖 Docker Compose 启动 PostgreSQL/Redis，除非传 `-SkipDocker`。
- 依赖 `tools/bge_server/.venv` 或系统 `python` 启动 BGE，除非传 `-SkipEmbedding`。
- 依赖 `npm.cmd` 启动前端。
- 可能在 `tmp/server` 下生成日志和 `import-spool`。
- 不提交、删除或修改数据库数据。

## 7. 测试

已执行语法验证：

```powershell
$path = "scripts\start-local-real.ps1"; $tokens=$null; $errors=$null; [System.Management.Automation.Language.Parser]::ParseFile((Resolve-Path $path), [ref]$tokens, [ref]$errors)
```

结果：无 parser errors。

已执行启动验证：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\start-local-real.ps1 -NoBrowser
```

结果：

```text
[14:18:54] starting postgres and redis with docker compose
[14:18:55] embedding already reachable: http://127.0.0.1:8000/healthz
[14:18:55] backend already reachable: http://127.0.0.1:8080
[14:18:55] frontend already reachable: http://127.0.0.1:5173
[14:18:55] ready: http://127.0.0.1:5173/questions
```

## 8. 风险

- 如果本地模型缓存不存在，BGE 离线启动会失败。
- 如果 `env1.txt` 缺少真实 LLM key，后端可启动但真实 LLM 调用会失败。
- 如果端口 8000、8080 或 5173 被占用，脚本会复用已可达服务；若端口被无效进程占用，等待健康检查会失败。

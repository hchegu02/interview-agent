# 真实 E2E 脚本端口与诊断修复

## 1. 变更概述

本次修复 `scripts/real_e2e.ps1` 在本地真实业务演练中的两个现实问题：

- Redis 不再固定依赖 Docker Compose 的 `6379:6379` 映射，默认使用 `redis://localhost:6479/0` 并按需启动临时 Redis 容器。
- Web server 默认 `18080` 被占用时，脚本会自动选择 `18081..18089` 的可用端口。
- server 启动过程中如果进程提前退出，脚本会立即输出 stdout/stderr 日志尾部，而不是继续等到健康检查超时。
- 顶层异常会输出失败原因、产物目录和 server 日志尾部，避免真实演练失败时只看到退出码。
- 新增分段跳过参数和 run log，调试 Web/SSE 时可以跳过迁移、reindex 和 CLI 真实 LLM demo。
- 修复 session detail URL 中 `$sessionID?user_id=...` 的 PowerShell 字符串插值歧义，避免 Web 流程完成后最后一步读取详情误报 400。
- 增强 server 清理逻辑：停止前刷新进程状态，停止后等待并记录未退出 warning。
- 修复 `$serverProcess` 在 `Invoke-Step` scriptblock 内赋值导致外层 `finally` 看不到进程对象的问题，避免 server 残留占用端口。

影响范围：仅真实演练脚本和 README 使用说明；不改变 Go 服务、API、数据库 schema、前端代码和正式运行配置。

## 2. 变更文件

- `scripts/real_e2e.ps1`
  - 新增 `RedisUrl`、`RedisContainerName` 参数。
  - 新增端口探测、Redis 启动和日志输出辅助函数。
  - Docker 启动阶段改为 Compose 只启动 PostgreSQL，Redis 由脚本按 `RedisUrl` 单独确保。
  - server 健康检查阶段增加进程提前退出检测和日志输出。
  - 顶层 `catch` 输出失败原因、产物目录和 server 日志尾部。
  - 新增 `SkipMigrations`、`SkipReindex`、`SkipCli`、`RunLog` 参数。
  - session detail URL 改为 `-f` 格式化拼接。
  - server 进程引用改用 `$script:serverProcess`，`Stop-Server` 增加进程状态刷新、等待结果检查和残留进程 warning。
- `README.md`
  - 更新真实完整演示中的 Redis URL 示例为 `redis://localhost:6479/0`。
  - 说明脚本默认避开 Windows 常见 `6379` 保留端口，并可自动避让 `18080`。
  - 说明 Web/SSE 调试可使用分段跳过参数。

## 3. 函数级说明

### `scripts/real_e2e.ps1` 参数区

- 新增 `RedisUrl`
  - 输入：优先使用 `$env:INTERVIEW_REDIS_URL`，否则默认 `redis://localhost:6479/0`。
  - 输出/副作用：后续写入当前进程 `$env:INTERVIEW_REDIS_URL`，供 Go server 使用。
  - 行为变化：真实演练默认不再要求宿主机 `6379` 可绑定。
- 新增 `RedisContainerName`
  - 输入：优先使用 `$env:REAL_E2E_REDIS_CONTAINER`，否则 `interview-agent-redis-real-e2e`。
  - 输出/副作用：当 Redis 不可达时，作为临时 Redis 容器名。
- 新增 `SkipMigrations`
  - 输入：开关参数。
  - 行为：跳过数据库 migration 应用阶段。
  - 适用：数据库已准备好、只想验证后续链路。
- 新增 `SkipReindex`
  - 输入：开关参数。
  - 行为：跳过真实 embedding reindex。
  - 适用：题库 embedding 已存在、避免重复调用本地 embedding 服务。
- 新增 `SkipCli`
  - 输入：开关参数。
  - 行为：跳过 CLI 真实 LLM demo。
  - 适用：只验证 Web/SSE 链路，避免重复消耗真实 LLM。
- 新增 `RunLog`
  - 输入：可选日志路径。
  - 输出/副作用：脚本步骤、失败原因和日志尾部写入该文件；为空时默认 `tmp/real-e2e/run-<timestamp>.log`。

### `Write-RunLog`

- 作用：把关键步骤同时输出到终端和 run log 文件。
- 输入：日志消息。
- 输出：终端一行日志。
- 副作用：追加写入 `runLog`。

### `Get-UrlPort`

- 作用：从 URL 中解析端口。
- 输入：`RawUrl` 和 `DefaultPort`。
- 输出：URL 中显式端口，或默认端口。
- 错误处理：URL 无法解析时抛出 `invalid URL`。
- 副作用：无。

### `Test-TcpPort`

- 作用：用 `TcpClient` 检查本机端口是否可连接。
- 输入：host、port 和超时时间。
- 输出：布尔值。
- 错误处理：连接异常返回 `false`。
- 副作用：短连接探测。

### `Wait-TcpPort`

- 作用：轮询等待端口变为可连接。
- 输入：host、port 和最大尝试次数。
- 输出：成功时无返回。
- 错误处理：超时后抛出 `$HostName:$Port did not become reachable`。
- 副作用：最多等待 `Attempts` 秒。

### `Write-LogTail`

- 作用：输出指定日志文件尾部，避免 server 启动失败时无诊断信息。
- 输入：日志路径和尾部行数。
- 输出：日志尾部内容。
- 错误处理：文件不存在则跳过。
- 副作用：向终端输出日志。

### `Resolve-BaseUrl`

- 作用：当用户没有显式设置 `BASE_URL`，且默认 `BaseUrl` 端口已被占用时，自动选择空闲端口。
- 输入：请求的 BaseUrl。
- 输出：原 BaseUrl 或 `18081..18089` 中的可用 BaseUrl。
- 错误处理：端口都不可用时抛出错误。
- 副作用：输出端口切换提示。

### `Ensure-Redis`

- 作用：确保 `$env:INTERVIEW_REDIS_URL` 指向的 Redis 可连接。
- 输入：Redis URL 和容器名。
- 输出：成功时 Redis 端口可连接。
- 错误处理：
  - 端口已可连接时直接复用。
  - 端口不可连接时，删除同名旧临时容器并 `docker run` 启动 Redis。
  - 启动后端口仍不可连接则抛错。
- 副作用：可能创建或替换名为 `interview-agent-redis-real-e2e` 的本地临时 Redis 容器。

### `Start PostgreSQL` 步骤

- 作用变化：从 `docker compose up -d postgres redis` 改为只启动 `postgres`。
- 原因：Compose Redis 固定映射 `6379:6379`，在 Windows 保留端口环境下会失败。
- 副作用：仍会启动/复用 Compose PostgreSQL。

### `Apply all migrations` 步骤

- 行为变化：受 `SkipMigrations` 控制。
- 副作用：未跳过时仍按文件名顺序应用所有 `*.up.sql`。

### `Reindex question bank with real embedding` 步骤

- 行为变化：受 `SkipReindex` 控制。
- 副作用：未跳过时仍调用 `cmd/reindex` 写入题库 embedding。

### `Run CLI real E2E demo` 步骤

- 行为变化：受 `SkipCli` 控制。
- 副作用：未跳过时仍调用真实 LLM 并写入 `tmp/demos/real-<timestamp>`。

### `Run real Web/SSE E2E smoke` 步骤

- 行为变化：session detail URL 从双引号内直接拼接改为 `"{0}/.../{1}?..." -f $BaseUrl, $sessionID`。
- 原因：PowerShell 中 `$sessionID?user_id=...` 存在变量边界歧义，可能导致 URL 中 session id 丢失并触发 400。
- 错误处理：继续复用 `Wait-SessionDetail` 的重试逻辑。

### `Ensure Redis` 步骤

- 作用：新增步骤，按 `RedisUrl` 确认 Redis 可用。
- 数据流：`RedisUrl` 参数 -> `$env:INTERVIEW_REDIS_URL` -> `Ensure-Redis` -> Go server。

### `Start real server` 步骤

- 行为变化：健康检查循环中先检查 `$serverProcess.HasExited`。
- 行为变化：`Start-Process` 结果写入 `$script:serverProcess`，确保 `finally` 中的 `Stop-Server` 能看到同一个进程对象。
- 错误处理：如果 server 提前退出，立即输出 server stdout/stderr 日志尾部并抛出 exit code。
- 目的：避免端口冲突等问题被隐藏成“server did not become healthy”。

### 顶层 `catch`

- 作用：捕获真实演练任意阶段的未处理异常。
- 输出：异常消息、artifact 目录、server stdout/stderr 尾部。
- 错误处理：输出诊断后重新抛出，保留非 0 退出码。

### `Stop-Server`

- 行为变化：对停止 server 的过程增加保护。
- 错误处理：
  - 使用 `$script:serverProcess` 读取 server 进程对象，避免 scriptblock 作用域丢失。
  - 停止前刷新进程状态。
  - 等待 5 秒仍未退出时输出 warning。
  - 停止失败时输出 warning，不覆盖真实业务失败原因。

## 4. 调用链

### Redis 链路

用户运行 `scripts/real_e2e.ps1`
-> 参数解析 `RedisUrl`
-> 设置 `$env:INTERVIEW_REDIS_URL`
-> `Invoke-Step "Ensure Redis"`
-> `Ensure-Redis`
-> `Test-TcpPort`
-> 必要时 `docker run redis:7-alpine`
-> Go server 启动时读取 `$env:INTERVIEW_REDIS_URL`。

### Web server 端口链路

用户运行 `scripts/real_e2e.ps1`
-> `Resolve-BaseUrl`
-> 必要时切换 BaseUrl 端口
-> 设置 `$env:INTERVIEW_SERVER_ADDR`
-> `Start-Process` 启动 `server-real-e2e.exe`
-> 轮询 `$BaseUrl/healthz`
-> 提前退出时 `Write-LogTail` 输出日志。

### 失败诊断链路

任意步骤抛错
-> 顶层 `catch`
-> 输出 `real e2e failed`
-> 输出 artifact 目录
-> `Write-LogTail` 输出 server 日志
-> 重新抛出异常。

### 分段调试链路

用户运行 `scripts/real_e2e.ps1 -SkipMigrations -SkipReindex -SkipCli`
-> 复用已有数据库和题库 embedding
-> 构建并启动 server
-> 只执行 Web/SSE smoke
-> run log 记录所有步骤。

### Session detail 链路

Web/SSE smoke 完成 report
-> 使用 format 拼接 detail URL
-> `Wait-SessionDetail`
-> `GET /api/interview/sessions/{sessionID}?user_id=real-e2e`
-> 校验 `status=completed` 和 report 存在。

## 5. 数据流

- Redis URL 来源：参数或环境变量，默认 `redis://localhost:6479/0`。
- Redis 端口：从 URL 解析后用于 TCP 探测和 Docker 端口映射。
- BaseUrl 来源：参数或环境变量，默认 `http://127.0.0.1:18080`。
- server 端口：从最终 BaseUrl 写入 `$env:INTERVIEW_SERVER_ADDR`。
- 日志路径：`tmp/real-e2e/server-<timestamp>.log` 和 `.err.log`。
- run log 路径：默认 `tmp/real-e2e/run-<timestamp>.log`，或由 `RunLog` 参数指定。
- 失败诊断：从当前运行的 `$outDir`、`$serverLog`、`$serverErr` 读取。
- session detail URL：由 BaseUrl 和 sessionID 格式化生成，避免 PowerShell 变量边界歧义。

## 6. 依赖与副作用

- 依赖 Docker CLI 启动 PostgreSQL 和必要时启动临时 Redis。
- 可能创建或替换本地 Redis 容器 `interview-agent-redis-real-e2e`。
- 不修改数据库 schema。
- 不修改真实 LLM key、embedding key 或用户级环境变量。
- 不改变 `docker-compose.yml`，避免影响其他开发流程。

## 7. 测试

已执行：

- PowerShell 语法检查：`[scriptblock]::Create((Get-Content -Raw "scripts/real_e2e.ps1"))`，通过。
- README/脚本引用测试：`go test ./... -run "TestRealDemoScriptsDocumentAndAssertRealChain" -count=1`，通过。
- Web/SSE 快速真实路径：
  - 命令使用 `-SkipDocker -SkipMigrations -SkipReindex -SkipCli`，复用已有 PG、Redis、BGE-M3 和真实 LLM key。
  - 结果：退出码 `0`。
  - run log：`tmp/real-e2e/run-fast-cleanup-20260607-212815.log`。
  - 关键日志：
    - `real e2e ok: artifacts in D:\Documents\New project\interview-agent\tmp\demos\real-20260607-212816`
    - `Stopping server process 130808`
  - 端口清理验证：脚本退出后 `http://127.0.0.1:18089/healthz` 不可访问，说明本轮 server 已释放。
- 残留进程清理：
  - 清理对象：此前失败轮次遗留的 9 个 `server-real-e2e.exe` 临时进程。
  - 验证结果：`18080..18089` 全部 down。

## 8. 风险

- 脚本默认 Redis 端口从 `6379` 变为 `6479`，只影响真实演练脚本；README 已同步说明。
- 如果本机 `6479` 也被占用，需要通过 `-RedisUrl` 显式指定其他端口。
- `Ensure-Redis` 会替换同名临时容器；容器名可通过 `-RedisContainerName` 或 `REAL_E2E_REDIS_CONTAINER` 改写。

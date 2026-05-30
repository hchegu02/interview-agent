param(
    [string]$ConfigPath = $(if ($env:CONFIG_PATH) { $env:CONFIG_PATH } else { "config/config.yaml.example" }),
    [string]$ScriptPath = $(if ($env:DEMO_SCRIPT) { $env:DEMO_SCRIPT } else { "testdata/demo/example.yaml" }),
    [string]$BaseUrl = $(if ($env:BASE_URL) { $env:BASE_URL } else { "http://127.0.0.1:18080" }),
    [string]$EmbeddingBaseUrl = $(if ($env:INTERVIEW_EMBEDDING_BASE_URL) { $env:INTERVIEW_EMBEDDING_BASE_URL } else { "http://127.0.0.1:8000/v1" }),
    [string]$EmbeddingModel = $(if ($env:INTERVIEW_EMBEDDING_MODEL) { $env:INTERVIEW_EMBEDDING_MODEL } else { "BAAI/bge-m3" }),
    [int]$EmbeddingDimension = $(if ($env:INTERVIEW_EMBEDDING_DIMENSION) { [int]$env:INTERVIEW_EMBEDDING_DIMENSION } else { 1024 }),
    [switch]$SkipDocker,
    [switch]$SkipWeb
)

$ErrorActionPreference = "Stop"

$root = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $root

$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$outDir = Join-Path $root "tmp\demos\real-$timestamp"
$serverDir = Join-Path $root "tmp\real-e2e"
$serverBin = Join-Path $serverDir "server-real-e2e.exe"
$serverLog = Join-Path $serverDir "server-$timestamp.log"
$serverErr = Join-Path $serverDir "server-$timestamp.err.log"
$serverProcess = $null

function Assert-Env {
    param([string]$Name)
    if ([string]::IsNullOrWhiteSpace([Environment]::GetEnvironmentVariable($Name))) {
        throw "$Name required"
    }
}

function Invoke-Step {
    param(
        [string]$Name,
        [scriptblock]$Body
    )
    Write-Host "==> $Name"
    & $Body
}

function Invoke-Checked {
    param(
        [string]$FilePath,
        [string[]]$ArgumentList
    )
    & $FilePath @ArgumentList
    if ($LASTEXITCODE -ne 0) {
        throw "$FilePath $($ArgumentList -join ' ') failed with exit code $LASTEXITCODE"
    }
}

function Invoke-Json {
    param(
        [string]$Method,
        [string]$Url,
        [object]$Body = $null
    )
    if ($null -eq $Body) {
        return Invoke-RestMethod -Method $Method -Uri $Url
    }
    return Invoke-RestMethod -Method $Method -Uri $Url -ContentType "application/json" -Body ($Body | ConvertTo-Json -Depth 16 -Compress)
}

function Read-SseEvent {
    param(
        [System.IO.StreamReader]$Reader,
        [int]$TimeoutSeconds = 30
    )
    $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
    $event = [ordered]@{ id = ""; event = ""; data = "" }
    while ([DateTime]::UtcNow -lt $deadline) {
        $task = $Reader.ReadLineAsync()
        $remainingMs = [int][Math]::Max(1, ($deadline - [DateTime]::UtcNow).TotalMilliseconds)
        if (-not $task.Wait($remainingMs)) {
            throw "timed out waiting for SSE event"
        }
        $line = $task.Result
        if ($null -eq $line) {
            throw "SSE stream closed"
        }
        if ($line -eq "") {
            if ($event.event -ne "" -or $event.data -ne "" -or $event.id -ne "") {
                return [pscustomobject]$event
            }
            continue
        }
        if ($line.StartsWith(":")) {
            continue
        }
        if ($line.StartsWith("id:")) {
            $event.id = $line.Substring(3).Trim()
            continue
        }
        if ($line.StartsWith("event:")) {
            $event.event = $line.Substring(6).Trim()
            continue
        }
        if ($line.StartsWith("data:")) {
            $piece = $line.Substring(5).Trim()
            if ($event.data -eq "") {
                $event.data = $piece
            } else {
                $event.data = $event.data + "`n" + $piece
            }
        }
    }
    throw "timed out waiting for SSE event"
}

function Wait-SessionDetail {
    param(
        [string]$Url,
        [int]$Attempts = 12
    )
    $lastError = ""
    for ($i = 0; $i -lt $Attempts; $i++) {
        try {
            return Invoke-RestMethod -Method Get -Uri $Url
        } catch {
            $lastError = $_.Exception.Message
            Start-Sleep -Seconds 1
        }
    }
    throw "session detail was not readable after $Attempts attempts: $lastError"
}

function Stop-Server {
    if ($null -ne $serverProcess -and -not $serverProcess.HasExited) {
        Stop-Process -Id $serverProcess.Id -Force -ErrorAction SilentlyContinue
        $serverProcess.WaitForExit(5000) | Out-Null
    }
}

try {
    Assert-Env "INTERVIEW_LLM_API_KEY"
    Assert-Env "INTERVIEW_EMBEDDING_API_KEY"

    if ([string]::IsNullOrWhiteSpace($env:INTERVIEW_POSTGRES_DSN)) {
        $env:INTERVIEW_POSTGRES_DSN = "postgres://interview:interview@localhost:5432/interview?sslmode=disable"
    }
    if ([string]::IsNullOrWhiteSpace($env:INTERVIEW_REDIS_URL)) {
        $env:INTERVIEW_REDIS_URL = "redis://localhost:6379/0"
    }

    $env:INTERVIEW_LLM_MODE = "real"
    $env:INTERVIEW_EMBEDDING_MODE = "real"
    $env:INTERVIEW_EMBEDDING_BASE_URL = $EmbeddingBaseUrl
    $env:INTERVIEW_EMBEDDING_MODEL = $EmbeddingModel
    $env:INTERVIEW_EMBEDDING_DIMENSION = [string]$EmbeddingDimension
    if ([string]::IsNullOrWhiteSpace($env:INTERVIEW_SERVER_ADDR)) {
        $baseUri = [Uri]$BaseUrl
        $env:INTERVIEW_SERVER_ADDR = ":$($baseUri.Port)"
    }

    if (-not $SkipDocker) {
        Invoke-Step "Start PostgreSQL and Redis" {
            Invoke-Checked "docker" @("compose", "up", "-d", "postgres", "redis")
            for ($i = 0; $i -lt 40; $i++) {
                & docker compose exec -T postgres pg_isready -U interview | Out-Null
                if ($LASTEXITCODE -eq 0) { return }
                Start-Sleep -Seconds 1
            }
            throw "postgres did not become ready"
        }
    }

    Invoke-Step "Apply all migrations 001-008" {
        $upFiles = Get-ChildItem -Path "migrations" -Filter "*.up.sql" | Sort-Object Name
        foreach ($file in $upFiles) {
            Invoke-Checked "docker" @("compose", "exec", "-T", "postgres", "psql", "-U", "interview", "-d", "interview", "-v", "ON_ERROR_STOP=1", "-f", "/docker-entrypoint-initdb.d/$($file.Name)")
        }
    }

    Invoke-Step "Reindex question bank with real embedding" {
        Invoke-Checked "go" @("run", "./cmd/reindex", "-seed", "seeds/question_bank.json", "-mode", "real", "-base-url", $EmbeddingBaseUrl, "-model", $EmbeddingModel, "-dim", [string]$EmbeddingDimension, "-dsn", $env:INTERVIEW_POSTGRES_DSN)
    }

    Invoke-Step "Verify RAG database has embedded active questions" {
        $count = (& docker compose exec -T postgres psql -U interview -d interview -Atc "SELECT count(*) FROM question_bank WHERE status='active' AND embedding_status='embedded' AND embedding IS NOT NULL;").Trim()
        if ([int]$count -le 0) {
            throw "expected embedded active questions, got $count"
        }
        $modelCount = (& docker compose exec -T postgres psql -U interview -d interview -Atc "SELECT count(*) FROM question_bank WHERE embedding_model='$EmbeddingModel';").Trim()
        if ([int]$modelCount -le 0) {
            throw "expected rows with embedding_model=$EmbeddingModel, got $modelCount"
        }
    }

    Invoke-Step "Run CLI real E2E demo" {
        Invoke-Checked "go" @("run", "./cmd/demo", "-config", $ConfigPath, "-script", $ScriptPath, "-out", $outDir)
        $runPath = Join-Path $outDir "run.json"
        $reportPath = Join-Path $outDir "report.md"
        if (-not (Test-Path $runPath)) { throw "run.json not created" }
        if (-not (Test-Path $reportPath)) { throw "report.md not created" }
        $run = Get-Content $runPath -Raw | ConvertFrom-Json
        if ($run.config.retriever -ne "pgvector") { throw "run.json.config.retriever = $($run.config.retriever), want pgvector" }
        if ($null -eq $run.session.report) { throw "run.json session.report missing" }
        if ($null -eq $run.session.report.overall_score) { throw "run.json session.report.overall_score missing" }
        if ($null -eq $run.session.report.skill_breakdown) { throw "run.json session.report.skill_breakdown missing" }
        if ($null -eq $run.session.report.transcript_analysis) { throw "run.json session.report.transcript_analysis missing" }
        if ($null -eq $run.session.report.drill_plan) { throw "run.json session.report.drill_plan missing" }
        if ($run.llm_calls.Count -le 0) { throw "run.json.llm_calls is empty" }
        $nodeNames = @($run.nodes | ForEach-Object { $_.node })
        foreach ($requiredNode in @("parse_jd", "parse_resume", "gap_analyze", "retrieve_rag", "pick_next", "report")) {
            if ($nodeNames -notcontains $requiredNode) {
                throw "run.json.nodes missing $requiredNode"
            }
        }
    }

    if (-not $SkipWeb) {
        Invoke-Step "Build server for real Web/SSE E2E" {
            New-Item -ItemType Directory -Force -Path $serverDir | Out-Null
            Invoke-Checked "go" @("build", "-o", $serverBin, "./cmd/server")
        }

        Invoke-Step "Start real server" {
            $serverProcess = Start-Process -FilePath $serverBin `
                -ArgumentList @("-config", $ConfigPath) `
                -RedirectStandardOutput $serverLog `
                -RedirectStandardError $serverErr `
                -WindowStyle Hidden `
                -PassThru
            for ($i = 0; $i -lt 60; $i++) {
                try {
                    Invoke-RestMethod -Method Get -Uri "$BaseUrl/healthz" | Out-Null
                    return
                } catch {
                    Start-Sleep -Seconds 1
                }
            }
            if (Test-Path $serverLog) { Get-Content $serverLog }
            if (Test-Path $serverErr) { Get-Content $serverErr }
            throw "server did not become healthy"
        }

        Invoke-Step "Run real Web/SSE E2E smoke" {
            Invoke-RestMethod -Method Get -Uri "$BaseUrl/readyz" | Out-Null
            Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/ping" | Out-Null
            $facets = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/question-bank/facets"
            if ($null -eq $facets.skill_categories) {
                throw "question-bank facets missing skill_categories"
            }

            $sessionID = "real-web-$timestamp"
            $start = Invoke-Json -Method Post -Url "$BaseUrl/api/interview/start" -Body @{
                session_id = $sessionID
                user_id = "real-e2e"
                jd_text = "需要 Go 后端工程师，熟悉 Redis、PostgreSQL、性能优化和稳定性建设。"
                resume_text = "三年 Go 后端经验，做过 Redis 缓存、PostgreSQL 慢查询优化、秒杀库存服务和 Prometheus 监控。"
            }
            if ($null -eq $start.question) { throw "interview/start did not return question" }

            $client = [System.Net.Http.HttpClient]::new()
            $stream = $null
            $reader = $null
            try {
                $request = [System.Net.Http.HttpRequestMessage]::new([System.Net.Http.HttpMethod]::Get, "$BaseUrl/api/interview/stream?session_id=$sessionID&user_id=real-e2e")
                $response = $client.SendAsync($request, [System.Net.Http.HttpCompletionOption]::ResponseHeadersRead).GetAwaiter().GetResult()
                if (-not $response.IsSuccessStatusCode) {
                    throw "SSE stream returned HTTP $([int]$response.StatusCode)"
                }
                if ($response.Content.Headers.ContentType.MediaType -ne "text/event-stream") {
                    throw "SSE content type was $($response.Content.Headers.ContentType)"
                }
                $stream = $response.Content.ReadAsStreamAsync().GetAwaiter().GetResult()
                $reader = [System.IO.StreamReader]::new($stream)
                $snapshot = Read-SseEvent -Reader $reader
                if ($snapshot.event -ne "snapshot") {
                    throw "first SSE event was $($snapshot.event), want snapshot"
                }

                $answers = @(
                    "G 是 goroutine，M 是 OS 线程，P 是调度上下文。P 持有本地可运行队列，M 绑定 P 后执行 G。P 空闲时会从全局队列或其他 P 偷任务。",
                    "Redis 缓存击穿可以用 singleflight 或互斥锁控制回源，雪崩用 TTL 抖动和预热，穿透用布隆过滤器和空值缓存。",
                    "PostgreSQL 慢查询先看 EXPLAIN ANALYZE，确认是否走索引、是否有 Seq Scan，再考虑复合索引、分区和连接池配置。",
                    "稳定性排查会先看错误率、P99 延迟、饱和度和下游依赖状态，再用日志、metrics 和 trace 缩小范围。",
                    "接口性能优化会先定位瓶颈，常见手段包括批量查询、缓存热点数据、异步化非核心路径和减少序列化开销。",
                    "Kafka 积压要看 consumer lag、单条耗时、批大小、失败重试和下游阻塞，再决定扩分区或增加消费者。",
                    "接口幂等会用业务唯一键、唯一索引和状态机，Redis 只做短期防重或加速，最终一致性落到数据库约束。",
                    "限流会区分全局、用户和接口维度，令牌桶允许突发，分布式场景用 Redis Lua 保证原子性。",
                    "发布风险通过灰度、金丝雀、配置开关和快速回滚控制，发布后盯错误率、延迟和核心业务指标。",
                    "可观测性要让日志、指标和 trace 通过 trace_id 串起来，关键节点记录耗时、错误分类和降级原因。",
                    "Go 内存上涨会用 heap profile、goroutine profile 和 trace 区分对象泄漏、协程泄漏和调度阻塞。",
                    "Redis 热 key 可以通过统计和采样定位，再用本地缓存、key 拆分、请求合并和预热治理。"
                )
                $latest = $start
                foreach ($answer in $answers) {
                    if ($null -ne $latest.report) { break }
                    $latest = Invoke-Json -Method Post -Url "$BaseUrl/api/interview/answer" -Body @{
                        session_id = $sessionID
                        user_id = "real-e2e"
                        answer = $answer
                    }
                }
                if ($null -eq $latest.report) {
                    for ($i = 0; $i -lt 6 -and $null -eq $latest.report; $i++) {
                        $latest = Invoke-Json -Method Post -Url "$BaseUrl/api/interview/answer" -Body @{
                            session_id = $sessionID
                            user_id = "real-e2e"
                            answer = "继续补充：我会结合指标、日志、链路追踪和压测结果定位问题，并说明方案取舍。"
                        }
                    }
                }
                if ($null -eq $latest.report) { throw "real web flow did not produce report within answer budget" }
                if ($null -eq $latest.report.overall_score) { throw "report.overall_score missing" }
                if ($null -eq $latest.report.skill_breakdown) { throw "report.skill_breakdown missing" }
                if ($null -eq $latest.report.transcript_analysis) { throw "report.transcript_analysis missing" }
                if ($null -eq $latest.report.drill_plan) { throw "report.drill_plan missing" }

                $seenProgress = $false
                for ($i = 0; $i -lt 5; $i++) {
                    try {
                        $event = Read-SseEvent -Reader $reader -TimeoutSeconds 2
                    } catch {
                        break
                    }
                    if ($event.event -in @("interview.progress", "graph.node.start", "graph.node.end", "interview.completed")) {
                        $seenProgress = $true
                        break
                    }
                }
                if (-not $seenProgress) {
                    Write-Host "SSE progress event not observed before report; validating persisted session instead"
                }
            } finally {
                if ($null -ne $reader) { $reader.Dispose() }
                if ($null -ne $stream) { $stream.Dispose() }
                if ($null -ne $client) { $client.Dispose() }
            }

            $detail = Wait-SessionDetail -Url "$BaseUrl/api/interview/sessions/$sessionID?user_id=real-e2e"
            if ($detail.status -ne "completed") { throw "session status = $($detail.status), want completed" }
            if ($null -eq $detail.report) { throw "session detail missing report" }

            $list = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/interview/sessions?user_id=real-e2e&limit=10"
            $found = $false
            foreach ($session in $list.sessions) {
                if ($session.session_id -eq $sessionID) { $found = $true }
            }
            if (-not $found) { throw "sessions list did not include $sessionID" }
        }
    }

    Write-Host "real e2e ok: artifacts in $outDir"
} finally {
    Stop-Server
}

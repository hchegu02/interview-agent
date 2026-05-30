param(
    [string]$BaseUrl = $(if ($env:BASE_URL) { $env:BASE_URL } else { "http://127.0.0.1:8080" }),
    [string]$ServerBin = $(if ($env:SERVER_BIN) { $env:SERVER_BIN } else { ".\bin\server" }),
    [string]$ConfigPath = $(if ($env:CONFIG_PATH) { $env:CONFIG_PATH } else { "config/config.yaml.example" }),
    [switch]$UseExistingServer = $($env:USE_EXISTING_SERVER -eq "1")
)

$ErrorActionPreference = "Stop"
$serverProcess = $null
$logPath = Join-Path ([System.IO.Path]::GetTempPath()) "interview-agent-sse-smoke.log"
$errPath = Join-Path ([System.IO.Path]::GetTempPath()) "interview-agent-sse-smoke.err.log"

function Stop-SmokeServer {
    if ($null -ne $serverProcess -and -not $serverProcess.HasExited) {
        Stop-Process -Id $serverProcess.Id -Force -ErrorAction SilentlyContinue
        $serverProcess.WaitForExit(3000) | Out-Null
    }
}

function Invoke-SmokeJson {
    param(
        [string]$Method,
        [string]$Url,
        [string]$Body = ""
    )
    if ($Body -eq "") {
        return Invoke-RestMethod -Method $Method -Uri $Url
    }
    return Invoke-RestMethod -Method $Method -Uri $Url -ContentType "application/json" -Body $Body
}

function Read-SseEvent {
    param(
        [System.IO.StreamReader]$Reader,
        [int]$TimeoutSeconds = 10
    )

    $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
    $event = [ordered]@{
        id = ""
        event = ""
        data = ""
    }

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

try {
    if (-not $UseExistingServer) {
        $oldLLMMode = $env:INTERVIEW_LLM_MODE
        $oldEmbeddingMode = $env:INTERVIEW_EMBEDDING_MODE
        $env:INTERVIEW_LLM_MODE = "mock"
        $env:INTERVIEW_EMBEDDING_MODE = "mock"

        $serverProcess = Start-Process -FilePath $ServerBin `
            -ArgumentList @("-config", $ConfigPath) `
            -RedirectStandardOutput $logPath `
            -RedirectStandardError $errPath `
            -WindowStyle Hidden `
            -PassThru

        $env:INTERVIEW_LLM_MODE = $oldLLMMode
        $env:INTERVIEW_EMBEDDING_MODE = $oldEmbeddingMode
    }

    $healthy = $false
    for ($i = 0; $i -lt 30; $i++) {
        try {
            Invoke-RestMethod -Method Get -Uri "$BaseUrl/healthz" | Out-Null
            $healthy = $true
            break
        } catch {
            Start-Sleep -Seconds 1
        }
    }
    if (-not $healthy) {
        Write-Error "server did not become healthy"
        if (-not $UseExistingServer) {
            if (Test-Path $logPath) { Get-Content $logPath }
            if (Test-Path $errPath) { Get-Content $errPath }
        }
        exit 1
    }

    $sessionID = "sse-smoke-" + ([DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds())
    $startBody = @{
        session_id = $sessionID
        user_id = "sse-smoke"
        jd_text = "需要 Go 后端工程师，熟悉 Redis 和并发编程"
        resume_text = "两年 Go 后端经验，做过 Redis 缓存服务"
    } | ConvertTo-Json -Compress
    $startResp = Invoke-SmokeJson -Method Post -Url "$BaseUrl/api/interview/start" -Body $startBody
    if ($null -eq $startResp.question) {
        throw "interview/start did not return a question"
    }

    $client = [System.Net.Http.HttpClient]::new()
    $stream = $null
    $reader = $null
    try {
        $streamUrl = "$BaseUrl/api/interview/stream?session_id=$sessionID&user_id=sse-smoke"
        $request = [System.Net.Http.HttpRequestMessage]::new([System.Net.Http.HttpMethod]::Get, $streamUrl)
        $response = $client.SendAsync($request, [System.Net.Http.HttpCompletionOption]::ResponseHeadersRead).GetAwaiter().GetResult()
        if (-not $response.IsSuccessStatusCode) {
            throw "SSE stream returned HTTP $([int]$response.StatusCode)"
        }
        if ($response.Content.Headers.ContentType.MediaType -ne "text/event-stream") {
            throw "SSE content type was $($response.Content.Headers.ContentType)"
        }

        $stream = $response.Content.ReadAsStreamAsync().GetAwaiter().GetResult()
        $reader = [System.IO.StreamReader]::new($stream)

        $snapshot = Read-SseEvent -Reader $reader -TimeoutSeconds 10
        if ($snapshot.event -ne "snapshot") {
            throw "first SSE event was $($snapshot.event), want snapshot"
        }
        if (-not $snapshot.data.Contains("`"session_id`":`"$sessionID`"")) {
            throw "snapshot did not include session id"
        }

        $answerBody = @{
            session_id = $sessionID
            user_id = "sse-smoke"
            answer = "G 是 goroutine，M 是线程，P 负责本地队列和调度。"
        } | ConvertTo-Json -Compress
        Invoke-SmokeJson -Method Post -Url "$BaseUrl/api/interview/answer" -Body $answerBody | Out-Null

        $seenEvent = $false
        for ($i = 0; $i -lt 20; $i++) {
            $event = Read-SseEvent -Reader $reader -TimeoutSeconds 10
            if ($event.event -in @("interview.progress", "interview.completed", "interview.failed", "session.updated", "session.completed", "graph.node.start", "graph.node.end", "graph.node.error")) {
                $seenEvent = $true
                break
            }
        }
        if (-not $seenEvent) {
            throw "SSE stream did not receive graph/session event after answer"
        }
    } finally {
        if ($null -ne $reader) { $reader.Dispose() }
        if ($null -ne $stream) { $stream.Dispose() }
        if ($null -ne $client) { $client.Dispose() }
    }

    Write-Host "sse smoke ok: start, stream snapshot, answer, stream event"
} finally {
    if (-not $UseExistingServer) {
        Stop-SmokeServer
    }
}

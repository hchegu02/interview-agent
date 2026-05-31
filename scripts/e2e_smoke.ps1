param(
    [string]$BaseUrl = $(if ($env:BASE_URL) { $env:BASE_URL } else { "http://127.0.0.1:8080" }),
    [string]$ServerBin = $(if ($env:SERVER_BIN) { $env:SERVER_BIN } else { ".\bin\server" }),
    [string]$ConfigPath = $(if ($env:CONFIG_PATH) { $env:CONFIG_PATH } else { "config/config.yaml.example" }),
    [string]$OutDir = $(if ($env:OUT_DIR) { $env:OUT_DIR } else { "" }),
    [switch]$UseExistingServer = $($env:USE_EXISTING_SERVER -eq "1")
)

$ErrorActionPreference = "Stop"
$startedAt = Get-Date
$timestamp = $startedAt.ToString("yyyyMMdd-HHmmss")
if ($OutDir -eq "") {
    $OutDir = Join-Path "tmp\e2e" $timestamp
}
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

$serverProcess = $null
$logPath = Join-Path $OutDir "server.out.log"
$errPath = Join-Path $OutDir "server.err.log"
$checks = [ordered]@{}

function Stop-E2EServer {
    if ($null -ne $serverProcess -and -not $serverProcess.HasExited) {
        Stop-Process -Id $serverProcess.Id -Force -ErrorAction SilentlyContinue
        $serverProcess.WaitForExit(3000) | Out-Null
    }
}

function Set-Check {
    param([string]$Name, [bool]$Pass, [string]$Detail = "")
    $checks[$Name] = [ordered]@{ pass = $Pass; detail = $Detail }
}

function Invoke-E2EJson {
    param([string]$Method, [string]$Url, [object]$Body = $null)
    if ($null -eq $Body) {
        return Invoke-RestMethod -Method $Method -Uri $Url
    }
    $json = $Body | ConvertTo-Json -Depth 8 -Compress
    return Invoke-RestMethod -Method $Method -Uri $Url -ContentType "application/json" -Body $json
}

function Invoke-E2EJsonWithRetry {
    param(
        [string]$Method,
        [string]$Url,
        [object]$Body,
        [int]$Attempts = 10,
        [int]$DelaySeconds = 1
    )
    $lastError = $null
    for ($i = 0; $i -lt $Attempts; $i++) {
        try {
            return Invoke-E2EJson -Method $Method -Url $Url -Body $Body
        } catch {
            $lastError = $_
            Start-Sleep -Seconds $DelaySeconds
        }
    }
    throw $lastError
}

function Read-SseEvent {
    param([System.IO.StreamReader]$Reader, [int]$TimeoutSeconds = 10)
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
        if ($line.StartsWith(":")) { continue }
        if ($line.StartsWith("id:")) {
            $event.id = $line.Substring(3).Trim()
        } elseif ($line.StartsWith("event:")) {
            $event.event = $line.Substring(6).Trim()
        } elseif ($line.StartsWith("data:")) {
            $piece = $line.Substring(5).Trim()
            if ($event.data -eq "") { $event.data = $piece } else { $event.data = $event.data + "`n" + $piece }
        }
    }
    throw "timed out waiting for SSE event"
}

function Write-Summary {
    param([string]$Status, [string]$ErrorMessage = "")
    $endedAt = Get-Date
    $summary = [ordered]@{
        status = $Status
        error = $ErrorMessage
        base_url = $BaseUrl
        started_at = $startedAt.ToString("o")
        ended_at = $endedAt.ToString("o")
        duration_ms = [int]($endedAt - $startedAt).TotalMilliseconds
        checks = $checks
    }
    $summaryPath = Join-Path $OutDir "summary.json"
    $summary | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $summaryPath -Encoding UTF8
    Write-Host "summary: $summaryPath"
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
    if (-not $healthy) { throw "server did not become healthy" }
    Set-Check "healthz" $true

    Invoke-RestMethod -Method Get -Uri "$BaseUrl/readyz" | Out-Null
    Set-Check "readyz" $true
    Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/ping" | Out-Null
    Set-Check "ping" $true

    $facets = Invoke-E2EJson -Method Get -Url "$BaseUrl/api/question-bank/facets"
    if ($null -eq $facets.skill_categories) { throw "question-bank facets missing skill_categories" }
    Set-Check "question_bank_facets" $true

    $questions = Invoke-E2EJson -Method Get -Url "$BaseUrl/api/question-bank?limit=5"
    if ($null -eq $questions.items -or $questions.items.Count -lt 1) { throw "question-bank list returned no items" }
    Set-Check "question_bank_list" $true "items=$($questions.items.Count)"

    $sessionID = "e2e-smoke-" + ([DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds())
    $userID = "e2e-smoke"
    $startResp = Invoke-E2EJson -Method Post -Url "$BaseUrl/api/interview/start" -Body @{
        session_id = $sessionID
        user_id = $userID
        jd_text = "Need Go backend engineer familiar with Redis, PostgreSQL and concurrency."
        resume_text = "Go backend engineer, built Redis cache and PostgreSQL services."
    }
    if ($null -eq $startResp.question) { throw "interview/start did not return question" }
    Set-Check "interview_start" $true

    $client = [System.Net.Http.HttpClient]::new()
    $stream = $null
    $reader = $null
    try {
        $streamUrl = "$BaseUrl/api/interview/stream?session_id=$sessionID&user_id=$userID"
        $request = [System.Net.Http.HttpRequestMessage]::new([System.Net.Http.HttpMethod]::Get, $streamUrl)
        $response = $client.SendAsync($request, [System.Net.Http.HttpCompletionOption]::ResponseHeadersRead).GetAwaiter().GetResult()
        if (-not $response.IsSuccessStatusCode) { throw "SSE HTTP $([int]$response.StatusCode)" }
        if ($response.Content.Headers.ContentType.MediaType -ne "text/event-stream") { throw "bad SSE content type" }
        $stream = $response.Content.ReadAsStreamAsync().GetAwaiter().GetResult()
        $reader = [System.IO.StreamReader]::new($stream)
        $snapshot = Read-SseEvent -Reader $reader -TimeoutSeconds 10
        if ($snapshot.event -ne "snapshot") { throw "first SSE event was $($snapshot.event)" }
        Set-Check "sse_snapshot" $true

        $answerResp = $null
        for ($round = 0; $round -lt 5; $round++) {
            $answerResp = Invoke-E2EJsonWithRetry -Method Post -Url "$BaseUrl/api/interview/answer" -Body @{
                session_id = $sessionID
                user_id = $userID
                answer = "G is goroutine, M is OS thread, P owns run queue and scheduling resources."
            }
            if ($null -ne $answerResp.report) {
                break
            }
        }
        if ($null -eq $answerResp.report) { throw "interview/answer did not return report within 5 rounds" }
        Set-Check "interview_answer_report" $true

        $seenEvent = ""
        for ($i = 0; $i -lt 20; $i++) {
            $event = Read-SseEvent -Reader $reader -TimeoutSeconds 10
            if ($event.event -ne "" -or $event.data -ne "") {
                $seenEvent = $event.event
                break
            }
        }
        if ($seenEvent -eq "") { throw "SSE did not receive event after answer" }
        Set-Check "sse_after_answer" $true "event=$seenEvent"
    } finally {
        if ($null -ne $reader) { $reader.Dispose() }
        if ($null -ne $stream) { $stream.Dispose() }
        if ($null -ne $client) { $client.Dispose() }
    }

    $detail = Invoke-E2EJson -Method Get -Url "$BaseUrl/api/interview/sessions/$sessionID`?user_id=$userID"
    if ($detail.session_id -ne $sessionID) { throw "session detail mismatch" }
    Set-Check "session_detail" $true

    $list = Invoke-E2EJson -Method Get -Url "$BaseUrl/api/interview/sessions?user_id=$userID&limit=10"
    $found = $false
    foreach ($s in $list.sessions) {
        if ($s.session_id -eq $sessionID) { $found = $true; break }
    }
    if (-not $found) { throw "session list missing smoke session" }
    Set-Check "session_list" $true

    $metrics = (Invoke-WebRequest -UseBasicParsing -Method Get -Uri "$BaseUrl/metrics").Content
    if (-not $metrics.Contains("# HELP") -and -not $metrics.Contains("interview_")) { throw "metrics endpoint did not return prometheus text" }
    Set-Check "metrics" $true

    Write-Summary "pass"
    Write-Host "e2e smoke ok"
} catch {
    Set-Check "fatal" $false $_.Exception.Message
    Write-Summary "fail" $_.Exception.Message
    throw
} finally {
    if (-not $UseExistingServer) {
        Stop-E2EServer
    }
}

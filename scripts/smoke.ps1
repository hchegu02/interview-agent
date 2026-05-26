param(
    [string]$BaseUrl = $(if ($env:BASE_URL) { $env:BASE_URL } else { "http://127.0.0.1:8080" }),
    [string]$ServerBin = $(if ($env:SERVER_BIN) { $env:SERVER_BIN } else { ".\bin\server" }),
    [string]$ConfigPath = $(if ($env:CONFIG_PATH) { $env:CONFIG_PATH } else { "config/config.yaml.example" }),
    [switch]$UseExistingServer = $($env:USE_EXISTING_SERVER -eq "1")
)

$ErrorActionPreference = "Stop"
$serverProcess = $null
$logPath = Join-Path ([System.IO.Path]::GetTempPath()) "interview-agent-smoke.log"
$errPath = Join-Path ([System.IO.Path]::GetTempPath()) "interview-agent-smoke.err.log"

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

try {
    if (-not $UseExistingServer) {
        $postgresDsn = $env:INTERVIEW_POSTGRES_DSN
        $oldLLMMode = $env:INTERVIEW_LLM_MODE
        $oldEmbeddingMode = $env:INTERVIEW_EMBEDDING_MODE
        $oldPostgresDSN = $env:INTERVIEW_POSTGRES_DSN
        $env:INTERVIEW_LLM_MODE = "mock"
        $env:INTERVIEW_EMBEDDING_MODE = "mock"
        $env:INTERVIEW_POSTGRES_DSN = $postgresDsn

        $serverProcess = Start-Process -FilePath $ServerBin `
            -ArgumentList @("-config", $ConfigPath) `
            -RedirectStandardOutput $logPath `
            -RedirectStandardError $errPath `
            -WindowStyle Hidden `
            -PassThru

        $env:INTERVIEW_LLM_MODE = $oldLLMMode
        $env:INTERVIEW_EMBEDDING_MODE = $oldEmbeddingMode
        $env:INTERVIEW_POSTGRES_DSN = $oldPostgresDSN
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

    Invoke-RestMethod -Method Get -Uri "$BaseUrl/readyz" | Out-Null
    Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/ping" | Out-Null

    $startBody = @{
        session_id = "smoke-1"
        user_id = "smoke"
        jd_text = "需要 Go 后端工程师，熟悉 Redis 和并发编程"
        resume_text = "两年 Go 后端经验，做过 Redis 缓存服务"
    } | ConvertTo-Json -Compress
    $startResp = Invoke-SmokeJson -Method Post -Url "$BaseUrl/api/interview/start" -Body $startBody
    if ($null -eq $startResp.question) {
        throw "interview/start did not return a question"
    }

    $getResp = Invoke-SmokeJson -Method Get -Url "$BaseUrl/api/interview/sessions/smoke-1"
    if ($getResp.session_id -ne "smoke-1") {
        throw "interview/sessions/:id did not return smoke session"
    }

    $answerBody = @{
        session_id = "smoke-1"
        user_id = "smoke"
        answer = "G 是 goroutine，M 是线程，P 负责本地队列和调度。"
    } | ConvertTo-Json -Compress
    $answerResp = Invoke-SmokeJson -Method Post -Url "$BaseUrl/api/interview/answer" -Body $answerBody
    if ($null -eq $answerResp.report) {
        throw "interview/answer did not return a report"
    }

    $listResp = Invoke-SmokeJson -Method Get -Url "$BaseUrl/api/interview/sessions?user_id=smoke&limit=10"
    $found = $false
    foreach ($session in $listResp.sessions) {
        if ($session.session_id -eq "smoke-1") {
            $found = $true
            break
        }
    }
    if (-not $found) {
        throw "interview/sessions list did not include smoke session"
    }

    Write-Host "smoke ok: healthz, readyz, api/ping, interview start/get/answer/list"
} finally {
    if (-not $UseExistingServer) {
        Stop-SmokeServer
    }
}

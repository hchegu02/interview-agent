param(
    [string]$BaseUrl = $(if ($env:BASE_URL) { $env:BASE_URL } else { "http://127.0.0.1:8080" }),
    [string]$ComposeFile = $(if ($env:COMPOSE_FILE) { $env:COMPOSE_FILE } else { "docker-compose.yml" }),
    [string]$Service = $(if ($env:CHAOS_REDIS_SERVICE) { $env:CHAOS_REDIS_SERVICE } else { "redis" }),
    [int]$ReadyAttempts = $(if ($env:CHAOS_READY_ATTEMPTS) { [int]$env:CHAOS_READY_ATTEMPTS } else { 30 }),
    [int]$ReadyDelaySeconds = $(if ($env:CHAOS_READY_DELAY_SECONDS) { [int]$env:CHAOS_READY_DELAY_SECONDS } else { 2 }),
    [switch]$DryRun = $(if ($env:CHAOS_DRY_RUN -eq "1") { $true } else { $false }),
    [switch]$SkipReadyCheck = $(if ($env:CHAOS_SKIP_READY_CHECK -eq "1") { $true } else { $false })
)

$ErrorActionPreference = "Stop"

$root = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $root

$timestamp = Get-Date -Format "yyyyMMdd-HHmmssfff"
$outDir = Join-Path $root "tmp\chaos\$timestamp"
New-Item -ItemType Directory -Force -Path $outDir | Out-Null
$summaryPath = Join-Path $outDir "summary.json"

function Invoke-ReadyCheck {
    param(
        [string]$Url,
        [int]$Attempts,
        [int]$DelaySeconds
    )

    if ($SkipReadyCheck) {
        return [ordered]@{
            ok = $true
            attempts = 0
            response = $null
            error = "skipped"
        }
    }

    $lastError = ""
    for ($i = 1; $i -le $Attempts; $i++) {
        try {
            $response = Invoke-RestMethod -Method Get -Uri "$Url/readyz" -TimeoutSec 5
            return [ordered]@{
                ok = $true
                attempts = $i
                response = $response
                error = ""
            }
        } catch {
            $lastError = $_.Exception.Message
            Start-Sleep -Seconds $DelaySeconds
        }
    }

    return [ordered]@{
        ok = $false
        attempts = $Attempts
        response = $null
        error = $lastError
    }
}

function Invoke-Compose {
    param([string[]]$Arguments)

    if ($DryRun) {
        return [ordered]@{
            command = "docker " + ($Arguments -join " ")
            exit_code = 0
            output = "dry-run: command not executed"
            dry_run = $true
        }
    }

    $output = & docker @Arguments 2>&1
    $exitCode = $LASTEXITCODE
    return [ordered]@{
        command = "docker " + ($Arguments -join " ")
        exit_code = $exitCode
        output = ($output -join "`n")
        dry_run = $false
    }
}

$startedAt = (Get-Date).ToUniversalTime().ToString("o")
$stopwatch = [System.Diagnostics.Stopwatch]::StartNew()
$before = Invoke-ReadyCheck -Url $BaseUrl -Attempts 1 -DelaySeconds 0
$restart = Invoke-Compose -Arguments @("compose", "-f", $ComposeFile, "restart", $Service)
$recoveryWatch = [System.Diagnostics.Stopwatch]::StartNew()
$after = Invoke-ReadyCheck -Url $BaseUrl -Attempts $ReadyAttempts -DelaySeconds $ReadyDelaySeconds
$recoveryWatch.Stop()
$stopwatch.Stop()
$finishedAt = (Get-Date).ToUniversalTime().ToString("o")

$checks = @(
    [ordered]@{
        name = "readyz_before_restart"
        ok = [bool]$before.ok
        attempts = $before.attempts
        error = $before.error
    },
    [ordered]@{
        name = "compose_restart"
        ok = ($restart.exit_code -eq 0)
        command = $restart.command
        dry_run = [bool]$restart.dry_run
        error = $(if ($restart.exit_code -eq 0) { "" } else { $restart.output })
    },
    [ordered]@{
        name = "readyz_after_restart"
        ok = [bool]$after.ok
        attempts = $after.attempts
        error = $after.error
    }
)
$failures = @($checks | Where-Object { -not $_.ok } | ForEach-Object { $_.name })
$ok = ($failures.Count -eq 0)
$summary = [ordered]@{
    kind = "redis_restart"
    status = $(if ($ok) { "pass" } else { "fail" })
    service = $Service
    base_url = $BaseUrl
    compose_file = $ComposeFile
    dry_run = [bool]$DryRun
    started_at = $startedAt
    finished_at = $finishedAt
    duration_ms = [int64]$stopwatch.ElapsedMilliseconds
    recovery_ms = [int64]$recoveryWatch.ElapsedMilliseconds
    output_dir = $outDir
    checks = $checks
    failures = $failures
    sessions_started = 0
    answers_completed = 0
    sse_reconnect_ok = $null
    before_readyz = $before
    restart = $restart
    after_readyz = $after
    ok = $ok
}

$summary | ConvertTo-Json -Depth 16 | Set-Content -Path $summaryPath -Encoding UTF8
Write-Host "summary: $summaryPath"
if (-not $ok) {
    exit 1
}

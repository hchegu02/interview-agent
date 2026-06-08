param(
    [string]$EnvFile = $(if ($env:INTERVIEW_ENV_FILE) { $env:INTERVIEW_ENV_FILE } else { "D:\Desktop\env1.txt" }),
    [ValidateSet("real", "mock")]
    [string]$LLMMode = $(if ($env:INTERVIEW_LLM_MODE) { $env:INTERVIEW_LLM_MODE } else { "real" }),
    [string]$BackendUrl = $(if ($env:BACKEND_URL) { $env:BACKEND_URL } else { "http://127.0.0.1:8080" }),
    [string]$FrontendUrl = $(if ($env:FRONTEND_URL) { $env:FRONTEND_URL } else { "http://127.0.0.1:5173" }),
    [string]$EmbeddingUrl = $(if ($env:INTERVIEW_EMBEDDING_BASE_URL) { $env:INTERVIEW_EMBEDDING_BASE_URL } else { "http://127.0.0.1:8000/v1" }),
    [string]$ServerBin = $(if ($env:SERVER_BIN) { $env:SERVER_BIN } else { "tmp\server\interview-server.exe" }),
    [switch]$SkipDocker,
    [switch]$SkipEmbedding,
    [switch]$NoBrowser
)

$ErrorActionPreference = "Stop"

$root = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $root

$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$logDir = Join-Path $root "tmp\server"
$spoolDir = Join-Path $logDir "import-spool"
New-Item -ItemType Directory -Force -Path $logDir | Out-Null
New-Item -ItemType Directory -Force -Path $spoolDir | Out-Null

function Write-Step {
    param([string]$Message)
    Write-Host ("[{0}] {1}" -f (Get-Date -Format "HH:mm:ss"), $Message)
}

function Import-EnvFile {
    param([string]$Path)
    if (-not (Test-Path -LiteralPath $Path)) {
        Write-Step "env file not found, skipping: $Path"
        return
    }
    Get-Content -LiteralPath $Path | ForEach-Object {
        $line = $_.Trim()
        if ($line -match '^\$env:([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$') {
            $name = $matches[1]
            $value = $matches[2].Trim()
            if (($value.StartsWith('"') -and $value.EndsWith('"')) -or ($value.StartsWith("'") -and $value.EndsWith("'"))) {
                $value = $value.Substring(1, $value.Length - 2)
            }
            Set-Item -Path "Env:$name" -Value $value
        }
    }
}

function Test-Url {
    param([string]$Url)
    try {
        Invoke-WebRequest -UseBasicParsing -Uri $Url -TimeoutSec 3 | Out-Null
        return $true
    } catch {
        return $false
    }
}

function Wait-Url {
    param(
        [string]$Url,
        [int]$Attempts = 45,
        [int]$DelaySeconds = 2
    )
    for ($i = 0; $i -lt $Attempts; $i++) {
        if (Test-Url -Url $Url) {
            return
        }
        Start-Sleep -Seconds $DelaySeconds
    }
    throw "$Url did not become reachable"
}

function Start-LoggedProcess {
    param(
        [string]$Name,
        [string]$FilePath,
        [string[]]$ArgumentList,
        [string]$WorkingDirectory
    )
    $outLog = Join-Path $logDir "$Name-$timestamp.out.log"
    $errLog = Join-Path $logDir "$Name-$timestamp.err.log"
    $process = Start-Process -FilePath $FilePath `
        -ArgumentList $ArgumentList `
        -WorkingDirectory $WorkingDirectory `
        -WindowStyle Hidden `
        -RedirectStandardOutput $outLog `
        -RedirectStandardError $errLog `
        -PassThru
    Write-Step "$Name pid=$($process.Id) logs=$outLog"
    return $process
}

function Ensure-ServerBin {
    param([string]$Path)
    if (Test-Path -LiteralPath $Path) {
        return
    }
    Write-Step "server binary missing, building: $Path"
    $dir = Split-Path -Parent $Path
    if (-not [string]::IsNullOrWhiteSpace($dir)) {
        New-Item -ItemType Directory -Force -Path $dir | Out-Null
    }
    & go build -o $Path ./cmd/server
    if ($LASTEXITCODE -ne 0) {
        throw "go build ./cmd/server failed"
    }
}

function Get-EmbeddingHealthUrl {
    param([string]$BaseUrl)
    $uri = [Uri]$BaseUrl
    return "{0}://{1}:{2}/healthz" -f $uri.Scheme, $uri.Host, $uri.Port
}

Import-EnvFile -Path $EnvFile

if ([string]::IsNullOrWhiteSpace($env:INTERVIEW_POSTGRES_DSN)) {
    $env:INTERVIEW_POSTGRES_DSN = "postgres://interview:interview@localhost:5432/interview?sslmode=disable"
}
if ([string]::IsNullOrWhiteSpace($env:INTERVIEW_REDIS_ADDR)) {
    $env:INTERVIEW_REDIS_ADDR = "localhost:6379"
}

$env:INTERVIEW_LLM_MODE = $LLMMode
$env:INTERVIEW_SERVER_ADDR = ":8080"
$env:INTERVIEW_EMBEDDING_MODE = "real"
$env:INTERVIEW_EMBEDDING_BASE_URL = $EmbeddingUrl
$env:INTERVIEW_EMBEDDING_MODEL = "BAAI/bge-m3"
$env:INTERVIEW_EMBEDDING_DIMENSION = "1024"
$env:INTERVIEW_EMBEDDING_API_KEY = $(if ($env:INTERVIEW_EMBEDDING_API_KEY) { $env:INTERVIEW_EMBEDDING_API_KEY } else { "dummy" })
$env:INTERVIEW_IMPORT_SPOOL_DIR = $spoolDir

if (-not $SkipDocker) {
    Write-Step "starting postgres and redis with docker compose"
    & docker compose up -d postgres redis
    if ($LASTEXITCODE -ne 0) {
        throw "docker compose up -d postgres redis failed"
    }
}

if (-not $SkipEmbedding) {
    $embeddingHealthUrl = Get-EmbeddingHealthUrl -BaseUrl $EmbeddingUrl
    if (Test-Url -Url $embeddingHealthUrl) {
        Write-Step "embedding already reachable: $embeddingHealthUrl"
    } else {
        $bgeDir = Join-Path $root "tools\bge_server"
        $python = Join-Path $bgeDir ".venv\Scripts\python.exe"
        if (-not (Test-Path -LiteralPath $python)) {
            $python = "python"
        }
        $env:HF_HUB_OFFLINE = "1"
        $env:TRANSFORMERS_OFFLINE = "1"
        Write-Step "starting local BGE-M3 embedding service"
        Start-LoggedProcess -Name "bge" -FilePath $python -ArgumentList @("-m", "uvicorn", "bge_server:app", "--host", "127.0.0.1", "--port", "8000") -WorkingDirectory $bgeDir | Out-Null
        Wait-Url -Url $embeddingHealthUrl -Attempts 60 -DelaySeconds 2
    }
}

if (Test-Url -Url "$BackendUrl/api/ping") {
    Write-Step "backend already reachable: $BackendUrl"
} else {
    Ensure-ServerBin -Path $ServerBin
    Write-Step "starting backend"
    Start-LoggedProcess -Name "backend" -FilePath (Resolve-Path $ServerBin) -ArgumentList @() -WorkingDirectory $root | Out-Null
    Wait-Url -Url "$BackendUrl/api/ping" -Attempts 45 -DelaySeconds 2
}

if (Test-Url -Url $FrontendUrl) {
    Write-Step "frontend already reachable: $FrontendUrl"
} else {
    Write-Step "starting frontend"
    Start-LoggedProcess -Name "web" -FilePath "npm.cmd" -ArgumentList @("run", "dev", "--prefix", "web", "--", "--port", "5173") -WorkingDirectory $root | Out-Null
    Wait-Url -Url $FrontendUrl -Attempts 45 -DelaySeconds 2
}

$questionBankUrl = "$FrontendUrl/questions"
Write-Step "ready: $questionBankUrl"

if (-not $NoBrowser) {
    Start-Process $questionBankUrl
}

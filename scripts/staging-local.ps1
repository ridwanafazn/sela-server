param(
    [ValidateSet("init", "migrate", "up", "down", "status", "logs", "scheduler", "smoke", "tunnel", "health", "usage", "backup", "restore-check")]
    [string]$Action = "up",
    [int]$TimeoutSeconds = 120,
    [string]$BaseUrl = "http://127.0.0.1:18080/api/v1"
)

$ErrorActionPreference = "Continue"
$composeFile = "compose.staging.yml"
$envFile = ".env.staging.local"

function Invoke-Docker {
    param([string[]]$DockerArgs)
    $previous = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    & docker @DockerArgs 2>&1 | ForEach-Object { Write-Host $_ }
    $code = $LASTEXITCODE
    $ErrorActionPreference = $previous
    return $code
}

function Assert-Exit([int]$code, [string]$message) {
    if ($code -ne 0) {
        throw "$message (exit code $code)"
    }
}

function Require-EnvFile {
    if (-not (Test-Path $envFile)) {
        Write-Host "Creating $envFile from .env.staging..."
        & (Join-Path $PSScriptRoot "staging-local-init.ps1")
    }
}

function Get-Ollama {
    $command = Get-Command ollama -ErrorAction SilentlyContinue
    if ($command) { return $command.Source }
    $fallback = Join-Path $env:LOCALAPPDATA "Programs\Ollama\ollama.exe"
    if (Test-Path $fallback) { return $fallback }
    throw "Ollama is not installed. Use the host-Ollama prerequisite in docs/backend/15-staging-without-gcp.md."
}

function Wait-Api {
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        try {
            $ready = Invoke-RestMethod -Uri "$BaseUrl/health/ready" -TimeoutSec 3
            if ($ready.status -eq "ok") {
                Write-Host "API ready at $BaseUrl"
                return
            }
        } catch {
            Start-Sleep -Seconds 2
        }
    }
    throw "API did not become ready within $TimeoutSeconds seconds"
}

switch ($Action) {
    "init" {
        Require-EnvFile
        $ollama = Get-Ollama
        Write-Host "Using host Ollama: $ollama"
        $previous = $ErrorActionPreference
        $ErrorActionPreference = "Continue"
        & $ollama list *> $null
        $listExit = $LASTEXITCODE
        if ($listExit -ne 0) {
            $ErrorActionPreference = $previous
            throw "Ollama is not running. Start the Ollama app, then rerun init."
        }
        & $ollama pull nomic-embed-text 2>&1 | Out-Null
        $pullExit = $LASTEXITCODE
        $ErrorActionPreference = $previous
        if ($pullExit -ne 0) { throw "Failed to pull nomic-embed-text" }
        Write-Host "Ollama embedding model ready."
    }
    "migrate" {
        Require-EnvFile
        $code = Invoke-Docker @("compose", "-f", $composeFile, "run", "--rm", "--entrypoint", "/usr/local/bin/sela-migrate", "api", "up")
        Assert-Exit $code "migration failed"
    }
    "up" {
        Require-EnvFile
        $code = Invoke-Docker @("compose", "-f", $composeFile, "up", "-d", "--build", "api", "worker", "scheduler")
        Assert-Exit $code "failed to start local staging"
        Wait-Api
        Write-Host ""
        Write-Host "Local staging is running:"
        Write-Host "  API: $BaseUrl"
        Write-Host "  Tunnel: ngrok http 18080"
        Write-Host "  Logs: .\scripts\staging-local.ps1 logs"
    }
    "down" {
        $code = Invoke-Docker @("compose", "-f", $composeFile, "down")
        Assert-Exit $code "failed to stop local staging"
    }
    "status" {
        $code = Invoke-Docker @("compose", "-f", $composeFile, "ps")
        Assert-Exit $code "failed to read compose status"
        try {
            $ready = Invoke-RestMethod -Uri "$BaseUrl/health/ready" -TimeoutSec 3
            Write-Host "health: $($ready.status)"
        } catch {
            Write-Host "health: unavailable"
        }
    }
    "logs" {
        Invoke-Docker @("compose", "-f", $composeFile, "logs", "-f", "--tail=100", "api", "worker", "scheduler") | Out-Null
    }
    "scheduler" {
        Require-EnvFile
        $code = Invoke-Docker @("compose", "-f", $composeFile, "run", "--rm", "scheduler", "-once")
        Assert-Exit $code "scheduler fan-out failed"
    }
    "smoke" {
        & (Join-Path $PSScriptRoot "local-smoke.ps1") `
            -BaseUrl $BaseUrl `
            -WorkerMode None `
            -JournalCount 3 `
            -AsyncWaitSeconds 90
    }
    "tunnel" {
        if (-not (Get-Command ngrok -ErrorAction SilentlyContinue)) {
            throw "ngrok is not installed"
        }
        ngrok http 18080
    }
    "health" {
        Require-EnvFile
        $code = Invoke-Docker @("compose", "-f", $composeFile, "run", "--rm", "--entrypoint", "/usr/local/bin/sela-ops", "api", "health")
        Assert-Exit $code "health check failed"
    }
    "usage" {
        Require-EnvFile
        $code = Invoke-Docker @("compose", "-f", $composeFile, "run", "--rm", "--entrypoint", "/usr/local/bin/sela-ops", "api", "usage")
        Assert-Exit $code "usage report failed"
    }
    "backup" {
        Require-EnvFile
        & (Join-Path $PSScriptRoot "db-backup.ps1") -EnvFile $envFile
    }
    "restore-check" {
        Require-EnvFile
        & (Join-Path $PSScriptRoot "backup-verify.ps1") -EnvFile $envFile
    }
}

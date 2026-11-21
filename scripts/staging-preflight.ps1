param(
    [string]$Project = "sela-513807",
    [string]$EnvFile = ".env.staging",
    [string]$Region = "asia-southeast1"
)

$ErrorActionPreference = "Continue"
$script:failures = 0
$script:warnings = 0

function Write-Pass([string]$message) {
    Write-Host "PASS  $message" -ForegroundColor Green
}

function Write-Fail([string]$message) {
    $script:failures++
    Write-Host "FAIL  $message" -ForegroundColor Red
}

function Write-Warn([string]$message) {
    $script:warnings++
    Write-Host "WARN  $message" -ForegroundColor Yellow
}

function Get-EnvMap([string]$path) {
    $map = @{}
    if (-not (Test-Path $path)) {
        return $map
    }
    Get-Content $path | ForEach-Object {
        if ($_ -match '^([^#=]+)=(.*)$') {
            $map[$matches[1]] = $matches[2]
        }
    }
    return $map
}

function Test-Gcloud {
    $command = Get-Command gcloud -ErrorAction SilentlyContinue
    if (-not $command) {
        Write-Fail "gcloud is not installed"
        return
    }
    Write-Pass "gcloud installed"

    $account = (& gcloud auth list --filter=status:ACTIVE --format="value(account)" 2>$null | Select-Object -First 1)
    if ([string]::IsNullOrWhiteSpace($account)) {
        Write-Fail "no active gcloud account"
    } else {
        Write-Pass "gcloud account=$account"
    }

    $configured = (& gcloud config get-value project 2>$null)
    if ($configured -ne $Project) {
        Write-Warn "gcloud project=$configured, expected $Project"
    } else {
        Write-Pass "gcloud project=$Project"
    }

    $billing = (& gcloud billing projects describe $Project --format="value(billingEnabled)" 2>$null)
    if ($billing -eq "True") {
        Write-Pass "billing enabled for $Project"
    } else {
        Write-Fail "billing disabled for $Project; staging deployment is blocked"
    }
}

function Test-Apis([string[]]$required) {
    $enabled = @(& gcloud services list --project=$Project --enabled --format="value(config.name)" 2>$null)
    foreach ($api in $required) {
        if ($enabled -contains $api) {
            Write-Pass "API enabled: $api"
        } else {
            Write-Fail "API disabled: $api"
        }
    }
}

function Test-Env([hashtable]$envMap) {
    if ($envMap.Count -eq 0) {
        Write-Fail "env file not found or empty: $EnvFile"
        return
    }
    Write-Pass "env file loaded: $EnvFile"

    $required = @(
        "DATABASE_URL",
        "DATABASE_URL_UNPOOLED",
        "JWT_ACCESS_SECRET",
        "RATE_LIMIT_SECRET",
        "AI_PROVIDER",
        "AI_BASE_URL",
        "AI_MODEL",
        "AI_API_KEY",
        "EMBEDDING_PROVIDER",
        "VERTEX_PROJECT_ID",
        "VERTEX_REGION",
        "VERTEX_EMBEDDING_MODEL",
        "EMBEDDING_DIMENSION"
    )

    foreach ($key in $required) {
        if (-not $envMap.ContainsKey($key) -or [string]::IsNullOrWhiteSpace($envMap[$key])) {
            Write-Fail "$key is missing or empty"
        } else {
            Write-Pass "$key is set"
        }
    }

    if ($envMap.ContainsKey("EMBEDDING_DIMENSION") -and $envMap["EMBEDDING_DIMENSION"] -ne "768") {
        Write-Fail "EMBEDDING_DIMENSION must be 768"
    }
    if ($envMap.ContainsKey("AI_PROVIDER") -and $envMap["AI_PROVIDER"] -ne "openai") {
        Write-Fail "AI_PROVIDER should be openai for staging"
    }
    if ($envMap.ContainsKey("EMBEDDING_PROVIDER") -and $envMap["EMBEDDING_PROVIDER"] -ne "vertex") {
        Write-Fail "EMBEDDING_PROVIDER should be vertex for staging"
    }

    if ($envMap.ContainsKey("SAFETY_RESOURCES_FILE") -and -not [string]::IsNullOrWhiteSpace($envMap["SAFETY_RESOURCES_FILE"])) {
        $safety = $envMap["SAFETY_RESOURCES_FILE"]
        if (Test-Path $safety) {
            Write-Pass "SAFETY_RESOURCES_FILE exists"
        } else {
            Write-Fail "SAFETY_RESOURCES_FILE does not exist: $safety"
        }
    } else {
        Write-Warn "SAFETY_RESOURCES_FILE is empty; embedded crisis resources remain unreviewed"
    }
}

function Test-Tooling {
    if (Get-Command docker -ErrorAction SilentlyContinue) {
        Write-Pass "docker installed"
    } else {
        Write-Fail "docker is not installed"
    }
    if (Get-Command psql -ErrorAction SilentlyContinue) {
        Write-Pass "psql installed"
    } else {
        Write-Warn "psql is not installed; migration/version checks will use the Go runner"
    }
}

Write-Host "== SELA staging preflight =="
Write-Host "project=$Project region=$Region env=$EnvFile"
Write-Host ""

$envMap = Get-EnvMap $EnvFile

Test-Tooling
Test-Gcloud
Test-Apis @(
    "run.googleapis.com",
    "artifactregistry.googleapis.com",
    "cloudbuild.googleapis.com",
    "secretmanager.googleapis.com",
    "cloudscheduler.googleapis.com",
    "aiplatform.googleapis.com"
)
Test-Env $envMap

Write-Host ""
Write-Host "== summary =="
Write-Host "failures=$script:failures warnings=$script:warnings"
if ($script:failures -gt 0) {
    exit 1
}

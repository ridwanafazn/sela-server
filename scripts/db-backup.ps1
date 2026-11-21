param(
    [string]$EnvFile = ".env.staging.local",
    [string]$OutputDir = "backups",
    [string]$Label = "sela"
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $EnvFile)) {
    throw "env file not found: $EnvFile"
}
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "docker is required for the PostgreSQL 17 pg_dump client"
}

$envMap = @{}
Get-Content $EnvFile | ForEach-Object {
    if ($_ -match '^([^#=]+)=(.*)$') { $envMap[$matches[1]] = $matches[2] }
}

$url = $envMap["DATABASE_URL_UNPOOLED"]
if ([string]::IsNullOrWhiteSpace($url)) {
    $url = $envMap["DATABASE_URL"]
}
if ([string]::IsNullOrWhiteSpace($url)) {
    throw "DATABASE_URL_UNPOOLED or DATABASE_URL is required in $EnvFile"
}

if (-not (Test-Path $OutputDir)) {
    New-Item -ItemType Directory -Path $OutputDir | Out-Null
}
$hostDir = (Resolve-Path $OutputDir).Path

$stamp = Get-Date -Format "yyyyMMdd-HHmmss"
$name = "$Label-$stamp.dump"
$file = Join-Path $hostDir $name

& docker run --rm `
    -v "${hostDir}:/backup" `
    pgvector/pgvector:pg17 `
    pg_dump --dbname=$url --format=custom --no-owner --no-privileges --file="/backup/$name"
if ($LASTEXITCODE -ne 0) {
    throw "pg_dump failed with exit code $LASTEXITCODE"
}

$size = (Get-Item $file).Length
Write-Host "backup created: $file ($([math]::Round($size / 1MB, 2)) MB)"

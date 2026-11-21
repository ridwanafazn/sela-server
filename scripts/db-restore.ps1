param(
    [Parameter(Mandatory = $true)]
    [string]$DumpFile,
    [Parameter(Mandatory = $true)]
    [string]$TargetUrl,
    [switch]$Force
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $DumpFile)) {
    throw "dump file not found: $DumpFile"
}
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "docker is required for the PostgreSQL 17 pg_restore client"
}
if (-not $Force) {
    throw "restore is destructive; pass -Force after verifying TargetUrl"
}
if ($TargetUrl -match "ep-.*neon\.tech" -and $TargetUrl -notmatch "restore|test") {
    throw "refusing to restore into a Neon production-looking database without an explicit restore/test name"
}

$hostDir = Split-Path -Parent (Resolve-Path $DumpFile).Path
$name = Split-Path -Leaf $DumpFile

& docker run --rm `
    -v "${hostDir}:/backup" `
    pgvector/pgvector:pg17 `
    pg_restore --dbname=$TargetUrl --clean --if-exists --no-owner --no-privileges "/backup/$name"
if ($LASTEXITCODE -ne 0) {
    throw "pg_restore failed with exit code $LASTEXITCODE"
}

Write-Host "restore completed into the target database"

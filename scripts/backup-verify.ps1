param(
    [string]$EnvFile = ".env.staging.local",
    [string]$OutputDir = "backups",
    [int]$Port = 55432
)

$ErrorActionPreference = "Stop"

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "docker is not installed"
}

$container = "sela-restore-check"
docker rm -f $container *> $null
docker run -d --name $container `
    -e POSTGRES_USER=sela `
    -e POSTGRES_PASSWORD=sela `
    -e POSTGRES_DB=sela_restore_check `
    -p "$Port`:5432" `
    pgvector/pgvector:pg17 | Out-Null
if ($LASTEXITCODE -ne 0) {
    throw "failed to start restore-check PostgreSQL container"
}

try {
    Write-Host "waiting for restore-check PostgreSQL..."
    $ready = $false
    for ($i = 0; $i -lt 60; $i++) {
        docker exec $container pg_isready -U sela -d sela_restore_check *> $null
        if ($LASTEXITCODE -eq 0) { $ready = $true; break }
        Start-Sleep -Seconds 1
    }
    if (-not $ready) { throw "restore-check PostgreSQL did not become ready" }

    & (Join-Path $PSScriptRoot "db-backup.ps1") -EnvFile $EnvFile -OutputDir $OutputDir -Label "restore-check"
    $dump = Get-ChildItem $OutputDir -Filter "restore-check-*.dump" | Sort-Object LastWriteTime -Descending | Select-Object -First 1
    if (-not $dump) { throw "backup file was not created" }

    docker cp $dump.FullName "${container}:/tmp/restore.dump" | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "failed to copy dump into restore-check container" }

    docker exec $container pg_restore `
        --dbname=postgres://sela:sela@127.0.0.1:5432/sela_restore_check `
        --clean --if-exists --no-owner --no-privileges /tmp/restore.dump
    if ($LASTEXITCODE -ne 0) { throw "pg_restore failed with exit code $LASTEXITCODE" }

    $version = docker exec $container psql -U sela -d sela_restore_check -tAc "select version from schema_migrations limit 1"
    if ($LASTEXITCODE -ne 0) { throw "restore-check schema version query failed" }

    $tables = docker exec $container psql -U sela -d sela_restore_check -tAc "select count(*) from information_schema.tables where table_schema='public'"
    if ($LASTEXITCODE -ne 0) { throw "restore-check table count query failed" }

    Write-Host "restore verified: schema_version=$($version.Trim()) public_tables=$($tables.Trim())"
}
finally {
    docker rm -f $container *> $null
    Write-Host "restore-check container removed"
}

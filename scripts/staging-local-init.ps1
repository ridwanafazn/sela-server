param(
    [string]$SourceEnv = ".env.staging",
    [string]$TargetEnv = ".env.staging.local",
    [switch]$Force
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $SourceEnv)) {
    throw "source env file not found: $SourceEnv"
}
if ((Test-Path $TargetEnv) -and -not $Force) {
    Write-Host "$TargetEnv already exists. Use -Force to replace it."
    exit 0
}

$overrides = @{
    APP_ENV                         = "staging"
    APP_PORT                        = "8080"
    AI_PROVIDER                     = "openai"
    AI_BASE_URL                     = "https://openrouter.ai/api/v1"
    AI_MODEL                        = "deepseek/deepseek-v4.1-flash-1017"
    EMBEDDING_PROVIDER              = "openai"
    EMBEDDING_BASE_URL              = "http://host.docker.internal:11434/v1"
    EMBEDDING_API_KEY               = "ollama"
    EMBEDDING_MODEL                 = "nomic-embed-text"
    EMBEDDING_DIMENSION             = "768"
    RATE_LIMIT_TRUSTED_PROXY_HOPS   = "0"
    LOG_FORMAT                      = "text"
    EMAIL_PROVIDER                  = "file"
    EMAIL_OUTBOX_DIR                = "/var/mail"
    REQUIRE_EMAIL_VERIFICATION      = "false"
}

$lines = Get-Content $SourceEnv
$seen = @{}
$result = New-Object System.Collections.Generic.List[string]

foreach ($line in $lines) {
    if ($line -match '^([^#=]+)=(.*)$') {
        $key = $matches[1]
        if ($overrides.ContainsKey($key)) {
            $result.Add("$key=$($overrides[$key])")
            $seen[$key] = $true
        } else {
            $result.Add($line)
        }
    } else {
        $result.Add($line)
    }
}

foreach ($key in $overrides.Keys) {
    if (-not $seen.ContainsKey($key)) {
        $result.Add("$key=$($overrides[$key])")
    }
}

$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
$targetPath = [System.IO.Path]::GetFullPath((Join-Path (Get-Location) $TargetEnv))
[System.IO.File]::WriteAllLines($targetPath, $result, $utf8NoBom)

$envMap = @{}
Get-Content $TargetEnv | ForEach-Object {
    if ($_ -match '^([^#=]+)=(.*)$') { $envMap[$matches[1]] = $matches[2] }
}

Write-Host "Created $TargetEnv (secrets are not printed)."
Write-Host "Model: $($envMap['AI_MODEL'])"
Write-Host "Embedding: $($envMap['EMBEDDING_PROVIDER']) / $($envMap['EMBEDDING_MODEL'])"
if ([string]::IsNullOrWhiteSpace($envMap['AI_API_KEY'])) {
    Write-Host "WARN: AI_API_KEY is empty. Fill it in $SourceEnv before starting." -ForegroundColor Yellow
}
if ([string]::IsNullOrWhiteSpace($envMap['SAFETY_RESOURCES_FILE'])) {
    Write-Host "WARN: SAFETY_RESOURCES_FILE is empty. Review crisis resources before real use." -ForegroundColor Yellow
}
Write-Host ""
Write-Host "Next:"
Write-Host "  .\scripts\staging-local.ps1 init"
Write-Host "  .\scripts\staging-local.ps1 migrate"
Write-Host "  .\scripts\staging-local.ps1 up"

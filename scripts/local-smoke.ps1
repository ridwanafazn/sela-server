param(
    [string]$BaseUrl = "http://localhost:8080/api/v1",
    [ValidateSet("Docker", "GCloud", "None")]
    [string]$WorkerMode = "Docker",
    [string]$Network = "sela-server_default",
    [string]$Image = "sela-server:v0.2.1-local",
    [string]$DatabaseUrl = "postgres://sela:sela@postgres:5432/sela?sslmode=disable",
    [string]$Project = "sela-513807",
    [string]$Region = "asia-southeast1",
    [string]$GcloudJob = "sela-worker",
    [ValidateSet("Fake", "Mock")]
    [string]$LlmMode = "Fake",
    [string]$MockBaseUrl = "http://sela-mock-llm:9090/v1",
    [int]$JournalCount = 1,
    [int]$AsyncWaitSeconds = 0,
    [switch]$SkipWorker,
    [switch]$KeepAccount
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

if ($SkipWorker) { $WorkerMode = "None" }

function New-IdempotencyKey { [guid]::NewGuid().ToString() }

function Get-Gcloud {
    $command = Get-Command gcloud -ErrorAction SilentlyContinue
    if ($command) { return $command.Source }
    $fallback = Join-Path $env:LOCALAPPDATA "Google\Cloud SDK\google-cloud-sdk\bin\gcloud.cmd"
    if (Test-Path $fallback) { return $fallback }
    throw "gcloud not found"
}

function Invoke-WorkerOnce {
    if ($WorkerMode -eq "None") {
        Write-Host "SKIP worker"
        return
    }
    if ($WorkerMode -eq "GCloud") {
        $gcloud = Get-Gcloud
        & $gcloud run jobs execute $GcloudJob --project=$Project --region=$Region --wait 2>&1
        if ($LASTEXITCODE -ne 0) { throw "gcloud run jobs execute failed" }
        Write-Host "PASS worker job ($GcloudJob)"
        return
    }
    $aiProvider = if ($LlmMode -eq "Mock") { "openai" } else { "fake" }
    $aiModel = if ($LlmMode -eq "Mock") { "mock-model" } else { "fake-chat" }
    $workerArgs = @(
        "run", "--rm", "--network", $Network,
        "-e", "APP_ENV=production",
        "-e", "DATABASE_URL=$DatabaseUrl",
        "-e", "JWT_ACCESS_SECRET=local-smoke-only-secret-0123456789abcdef",
        "-e", "JWT_ISSUER=sela-api",
        "-e", "JWT_AUDIENCE=sela-mobile",
        "-e", "RATE_LIMIT_SECRET=local-smoke-rate-limit-0123456789abcdef",
        "-e", "AI_PROVIDER=$aiProvider",
        "-e", "AI_MODEL=$aiModel",
        "-e", "EMBEDDING_PROVIDER=fake",
        "-e", "EMBEDDING_MODEL=fake-embedding-768"
    )
    if ($LlmMode -eq "Mock") {
        $workerArgs += @("-e", "AI_BASE_URL=$MockBaseUrl", "-e", "AI_API_KEY=test")
    }
    $workerArgs += @("--entrypoint", "/usr/local/bin/sela-worker", $Image, "-once")
    $output = & docker @workerArgs 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "worker -once failed: $output"
    }
    Write-Host "PASS worker -once"
    $output | Select-Object -Last 3 | ForEach-Object { Write-Host "  $_" }
}

Write-Host "== SELA local container smoke =="

$live = Invoke-RestMethod -Uri "$BaseUrl/health/live" -TimeoutSec 10
if ($live.status -ne "ok") { throw "live health is not ok" }
Write-Host "PASS health/live"

$ready = Invoke-RestMethod -Uri "$BaseUrl/health/ready" -TimeoutSec 10
if ($ready.status -ne "ok" -or $ready.checks.database -ne "ok" -or $ready.checks.migrations -ne "ok") {
    throw "readiness is not ok: $($ready | ConvertTo-Json -Compress)"
}
Write-Host "PASS health/ready"

$email = "smoke+$([DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds())@example.com"
$password = "LocalSmoke!2026"
$registerBody = @{
    email        = $email
    password     = $password
    display_name = "Local Smoke"
} | ConvertTo-Json

$auth = Invoke-RestMethod -Method Post -Uri "$BaseUrl/auth/register" -ContentType "application/json" -Body $registerBody
$token = $auth.data.access_token
$refresh = $auth.data.refresh_token
if (-not $token) { throw "register did not return an access token" }
Write-Host "PASS register ($($auth.data.user.id))"

$headers = @{ Authorization = "Bearer $token" }

$schedule = Invoke-RestMethod -Uri "$BaseUrl/insight-schedule" -Headers $headers
if ($schedule.data.enabled) { throw "initial insight schedule is enabled" }
$enabledSchedule = Invoke-RestMethod -Method Put -Uri "$BaseUrl/insight-schedule" -Headers $headers -ContentType "application/json" -Body (@{
    enabled = $true
    cadence = "weekly"
} | ConvertTo-Json)
if (-not $enabledSchedule.data.enabled -or $enabledSchedule.data.cadence -ne "weekly") {
    throw "insight schedule did not enable"
}
$disabledSchedule = Invoke-RestMethod -Method Delete -Uri "$BaseUrl/insight-schedule" -Headers $headers
if ($disabledSchedule.data.enabled) { throw "insight schedule did not disable" }
Write-Host "PASS insight schedule lifecycle"

$consent = Invoke-RestMethod -Uri "$BaseUrl/memory-consent" -Headers $headers
if ($consent.data.granted) { throw "initial sensitive memory consent is granted" }
$grantedConsent = Invoke-RestMethod -Method Put -Uri "$BaseUrl/memory-consent" -Headers $headers -ContentType "application/json" -Body (@{
    granted = $true
} | ConvertTo-Json)
if (-not $grantedConsent.data.granted) { throw "sensitive memory consent did not grant" }
$revokedConsent = Invoke-RestMethod -Method Put -Uri "$BaseUrl/memory-consent" -Headers $headers -ContentType "application/json" -Body (@{
    granted = $false
} | ConvertTo-Json)
if ($revokedConsent.data.granted) { throw "sensitive memory consent did not revoke" }
Write-Host "PASS memory consent lifecycle"

$contents = @(
    "Hari ini saya merasa tenang setelah berjalan pagi. Saya ingin menjaga kebiasaan tidur lebih awal dan lebih sabar menghadapi pekerjaan.",
    "Saya sempat cemas soal pekerjaan, lalu mencoba menarik napas dan memilah mana yang bisa saya kendalikan.",
    "Malam ini saya bersyukur bisa makan malam dengan tenang dan membaca sebentar sebelum tidur."
)

for ($i = 1; $i -le $JournalCount; $i++) {
    $content = $contents[($i - 1) % $contents.Count]
    $journal = Invoke-RestMethod -Method Post -Uri "$BaseUrl/journals" -ContentType "application/json" -Body (@{
        title   = "Local smoke entry $i"
        content = $content
        status  = "active"
    } | ConvertTo-Json) -Headers ($headers + @{ "Idempotency-Key" = (New-IdempotencyKey) })
    $journalId = $journal.data.id
    if (-not $journalId) { throw "journal create did not return an id" }
    Write-Host "PASS journal create $i ($journalId)"
}

$list = Invoke-RestMethod -Uri "$BaseUrl/journals" -Headers $headers
if ($list.data.Count -lt $JournalCount) { throw "journal list is missing entries" }
Write-Host "PASS journal list ($($list.data.Count))"

if (-not $SkipWorker) {
    if ($WorkerMode -eq "None" -and $AsyncWaitSeconds -gt 0) {
        $deadline = (Get-Date).AddSeconds($AsyncWaitSeconds)
        do {
            $memories = Invoke-RestMethod -Uri "$BaseUrl/memories" -Headers $headers
            if ($memories.data.Count -gt 0) { break }
            Start-Sleep -Seconds 2
        } while ((Get-Date) -lt $deadline)
        Write-Host "PASS memories list ($($memories.data.Count) candidates; external worker)"
    } else {
        Invoke-WorkerOnce
        $memories = Invoke-RestMethod -Uri "$BaseUrl/memories" -Headers $headers
        $memorySource = if ($LlmMode -eq "Mock") { "mock provider" } else { "fake provider extracts none" }
        Write-Host "PASS memories list ($($memories.data.Count) candidates; $memorySource)"
    }
}

$conversation = Invoke-RestMethod -Method Post -Uri "$BaseUrl/conversations" -ContentType "application/json" -Body (@{
    title = "Local smoke conversation"
} | ConvertTo-Json) -Headers ($headers + @{ "Idempotency-Key" = (New-IdempotencyKey) })
$conversationId = $conversation.data.id
if (-not $conversationId) { throw "conversation create did not return an id" }
Write-Host "PASS conversation create ($conversationId)"

$sendHeaders = $headers + @{ "Idempotency-Key" = (New-IdempotencyKey) }
$send = Invoke-RestMethod -Method Post -Uri "$BaseUrl/conversations/$conversationId/messages" -Headers $sendHeaders -ContentType "application/json" -Body (@{
    content = "Saya ingin tidur lebih awal malam ini."
} | ConvertTo-Json)
if (-not $send.data.assistant_message.content) { throw "companion reply is empty" }
Write-Host "PASS companion reply"

$insightHeaders = $headers + @{ "Idempotency-Key" = (New-IdempotencyKey) }
$accepted = Invoke-RestMethod -Method Post -Uri "$BaseUrl/insights" -Headers $insightHeaders -ContentType "application/json" -Body (@{
    period = "weekly"
} | ConvertTo-Json)
$jobId = $accepted.data.job_id
if (-not $jobId) { throw "insight request did not return a job id" }
Write-Host "PASS insight accepted ($jobId)"

if (-not $SkipWorker) {
    if ($WorkerMode -eq "None" -and $AsyncWaitSeconds -gt 0) {
        $deadline = (Get-Date).AddSeconds($AsyncWaitSeconds)
        do {
            $job = Invoke-RestMethod -Uri "$BaseUrl/insight-jobs/$jobId" -Headers $headers
            if ($job.data.status -in @("completed", "failed")) { break }
            Start-Sleep -Seconds 2
        } while ((Get-Date) -lt $deadline)
    } else {
        Invoke-WorkerOnce
        $job = Invoke-RestMethod -Uri "$BaseUrl/insight-jobs/$jobId" -Headers $headers
    }
    Write-Host "INFO insight job status=$($job.data.status) outcome=$($job.data.outcome) insight_id=$($job.data.insight_id)"
    if ($job.data.status -eq "failed") {
        Write-Host "INFO insight failure is expected with the fake provider; the integration suite covers the real insight path."
    }
}

if (-not $KeepAccount) {
    $deleteHeaders = $headers + @{ "Idempotency-Key" = (New-IdempotencyKey) }
    Invoke-RestMethod -Method Delete -Uri "$BaseUrl/me" -Headers $deleteHeaders -ContentType "application/json" -Body (@{
        password     = $password
        confirmation = "DELETE"
    } | ConvertTo-Json) | Out-Null
    Write-Host "PASS account delete"

    if ($WorkerMode -ne "None") { Invoke-WorkerOnce }

    $loginBody = @{ email = $email; password = $password } | ConvertTo-Json
    $loginStatus = 0
    try {
        $login = Invoke-WebRequest -Method Post -Uri "$BaseUrl/auth/login" -ContentType "application/json" -Body $loginBody
        $loginStatus = $login.StatusCode
    } catch {
        if ($_.Exception.Response) {
            $loginStatus = [int]$_.Exception.Response.StatusCode
        } else {
            throw
        }
    }
    if ($loginStatus -ne 401) { throw "deleted account login returned $loginStatus, expected 401" }
    Write-Host "PASS deleted account cannot log in"
}

Write-Host "== smoke complete =="

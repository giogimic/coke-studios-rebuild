# ==============================================================================
# Coke Studios 1:1 Complete Rebuild - Windows Docker Setup & Auto-Updater
# Target OS: Windows (Compatibility) / Docker Desktop
# Default Launch Port: 24902
# Scalable Database: MariaDB 11
# ==============================================================================
param (
    [switch]$Update
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $ScriptDir

$Port = if ($env:PORT) { $env:PORT } else { "24902" }
$Mode = if ($Update) { "UPDATE & REBUILD" } else { "FRESH LAUNCH" }

Write-Host "====================================================================" -ForegroundColor Cyan
Write-Host "    COKE STUDIOS & COKE MUSIC 1:1 REBUILD - DOCKER SETUP" -ForegroundColor Red
Write-Host "====================================================================" -ForegroundColor Cyan
Write-Host " Mode:         $Mode" -ForegroundColor Yellow
Write-Host " Ingress Port: $Port" -ForegroundColor Yellow
Write-Host " Database:     MariaDB 11 (Docker Container)" -ForegroundColor Yellow
Write-Host " Workspace:    $ScriptDir" -ForegroundColor Yellow
Write-Host "====================================================================" -ForegroundColor Cyan

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    Write-Host "[ERROR] Docker is not installed or not in PATH. Please install Docker Desktop for Windows." -ForegroundColor Red
    exit 1
}

# Generate .env if missing
if (-not (Test-Path ".env")) {
    Write-Host "[*] Generating .env configuration..." -ForegroundColor Gray
    @"
PORT=$Port
DB_NAME=coke_studios
DB_USER=coke
DB_PASS=coke_secret
DB_ROOT_PASSWORD=coke_root_secret
DATABASE_URL=coke:coke_secret@tcp(mariadb:3306)/coke_studios?parseTime=true
"@ | Out-File -FilePath ".env" -Encoding utf8
}

if ($Update) {
    Write-Host "[*] STEP 1/4: Pulling latest changes from Git..." -ForegroundColor Cyan
    if (Test-Path ".git") {
        git pull origin master
    }
    Write-Host "[*] STEP 2/4: Rebuilding Docker images..." -ForegroundColor Cyan
    docker compose build --pull
    Write-Host "[*] STEP 3/4: Restarting containers..." -ForegroundColor Cyan
    docker compose down
    docker compose up -d
    Write-Host "[*] STEP 4/4: Database migrations applied automatically." -ForegroundColor Cyan
} else {
    Write-Host "[*] Launching MariaDB & Coke Studios via Docker Compose..." -ForegroundColor Cyan
    docker compose up -d --build
}

Write-Host "`n[*] Waiting for Coke Studios on http://127.0.0.1:$Port/health..." -ForegroundColor Gray
$Ready = $false
for ($i = 0; $i -lt 30; $i++) {
    try {
        $res = Invoke-RestMethod -Uri "http://127.0.0.1:$Port/health" -TimeoutSec 2 -ErrorAction Stop
        if ($res) {
            $Ready = $true
            break
        }
    } catch {
        Start-Sleep -Seconds 2
        Write-Host -NoNewline "."
    }
}
Write-Host ""

if ($Ready) {
    Write-Host "====================================================================" -ForegroundColor Green
    Write-Host "  SUCCESS: COKE STUDIOS IS LIVE AND HEALTHY!" -ForegroundColor Green
    Write-Host "====================================================================" -ForegroundColor Green
    Write-Host "  Official MyCoke Game Portal: http://127.0.0.1:$Port/" -ForegroundColor White
    Write-Host "  Direct Game Client:         http://127.0.0.1:$Port/game/?user=Adam" -ForegroundColor White
    Write-Host "  Music Mixer Studio:         http://127.0.0.1:$Port/mixer/" -ForegroundColor White
    Write-Host "====================================================================" -ForegroundColor Green
    Write-Host "  To update at any time with zero data loss, run:" -ForegroundColor Cyan
    Write-Host "  .\setup.ps1 -Update" -ForegroundColor Yellow
    Write-Host "====================================================================" -ForegroundColor Green
} else {
    Write-Host "[*] Containers are initializing in the background. Check logs with 'docker compose logs -f'." -ForegroundColor Yellow
    Write-Host "    Then visit: http://127.0.0.1:$Port/" -ForegroundColor White
}

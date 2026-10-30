# ==============================================================================
# Coke Studios 1:1 Complete Rebuild - Windows Docker Setup & Auto-Updater
# Target OS: Windows (Compatibility) / Docker Desktop
# Default Launch Port: 24902
# Scalable Database: MariaDB 11
# ==============================================================================
param (
    [switch]$Update,
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$RemainingArgs
)

if (-not $Update -and $RemainingArgs) {
    foreach ($arg in $RemainingArgs) {
        if ($arg -match '(?i)^(--|-)?update$') {
            $Update = $true
            break
        }
    }
}

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $ScriptDir

# Helper: Test if a TCP port is currently open / in-use
function Test-PortInUse([int]$PortNumber) {
    try {
        $conns = Get-NetTCPConnection -LocalPort $PortNumber -ErrorAction SilentlyContinue
        if ($conns -and $conns.Count -gt 0) { return $true }
    } catch {}
    try {
        $client = New-Object System.Net.Sockets.TcpClient
        $asyncResult = $client.BeginConnect("127.0.0.1", $PortNumber, $null, $null)
        $success = $asyncResult.AsyncWaitHandle.WaitOne(200, $false)
        if ($success) {
            $client.EndConnect($asyncResult)
            $client.Close()
            return $true
        }
        $client.Close()
    } catch {}
    return $false
}

function Find-NextFreePort([int]$StartingPort) {
    $p = $StartingPort
    while (Test-PortInUse $p) {
        $p++
    }
    return $p
}

# Defaults
$Port = if ($env:PORT) { $env:PORT } else { "24902" }
$BackendPort = if ($env:BACKEND_PORT) { $env:BACKEND_PORT } else { "8080" }
$DbPort = if ($env:DB_PORT) { $env:DB_PORT } else { "3306" }
$CokePhasePort = if ($env:COKEPHASE_PORT) { $env:COKEPHASE_PORT } else { "8081" }
$MusicMixerPort = if ($env:MUSICMIXER_PORT) { $env:MUSICMIXER_PORT } else { "8082" }

# Load existing configuration if .env exists
if (Test-Path ".env") {
    Get-Content ".env" | ForEach-Object {
        $line = $_.Trim()
        if ($line -and -not $line.StartsWith("#") -and $line.Contains("=")) {
            $idx = $line.IndexOf("=")
            $k = $line.Substring(0, $idx).Trim()
            $v = $line.Substring($idx + 1).Trim()
            if ($k -eq "PORT") { $Port = $v }
            if ($k -eq "BACKEND_PORT") { $BackendPort = $v }
            if ($k -eq "DB_PORT") { $DbPort = $v }
            if ($k -eq "COKEPHASE_PORT") { $CokePhasePort = $v }
            if ($k -eq "MUSICMIXER_PORT") { $MusicMixerPort = $v }
        }
    }
}

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

# Interactive Port Prompt (checks for conflicts on 8080, 3306, etc.)
if (-not $Update) {
    if ([Environment]::UserInteractive -and -not [Console]::IsInputRedirected) {
        Write-Host "`n--------------------------------------------------------------------" -ForegroundColor Gray
        Write-Host "                     PORT SELECTION & CHECK" -ForegroundColor Cyan
        Write-Host "  Default ports (like 8080 or 3306) can conflict with local web" -ForegroundColor Gray
        Write-Host "  servers, IIS, or MySQL. We verify availability before launch." -ForegroundColor Gray
        Write-Host "--------------------------------------------------------------------" -ForegroundColor Gray

        # 1. Primary Ingress Portal Port
        $SuggPort = $Port
        if (Test-PortInUse ([int]$Port)) {
            $SuggPort = (Find-NextFreePort ([int]$Port)).ToString()
            Write-Host "  [!] Port $Port is IN USE. Suggested free port: $SuggPort" -ForegroundColor Yellow
        }
        $InPort = Read-Host "  [1/5] Primary Portal Port [default: $SuggPort]"
        if ($InPort) { $Port = $InPort } else { $Port = $SuggPort }

        # 2. Go Backend Port
        $SuggBackend = $BackendPort
        if (Test-PortInUse ([int]$BackendPort)) {
            $SuggBackend = (Find-NextFreePort 8085).ToString()
            Write-Host "  [!] Port $BackendPort is ALREADY IN USE! Suggested free port: $SuggBackend" -ForegroundColor Yellow
        }
        $InBackend = Read-Host "  [2/5] Standalone Go Backend Port [default: $SuggBackend]"
        if ($InBackend) { $BackendPort = $InBackend } else { $BackendPort = $SuggBackend }

        # 3. MariaDB Host Port
        $SuggDb = $DbPort
        if (Test-PortInUse ([int]$DbPort)) {
            $SuggDb = (Find-NextFreePort 3307).ToString()
            Write-Host "  [!] Port $DbPort (MySQL) is ALREADY IN USE! Suggested free port: $SuggDb" -ForegroundColor Yellow
        }
        $InDb = Read-Host "  [3/5] MariaDB Host Port [default: $SuggDb]"
        if ($InDb) { $DbPort = $InDb } else { $DbPort = $SuggDb }

        # 4. CokePhase Port
        $SuggCp = $CokePhasePort
        if (Test-PortInUse ([int]$CokePhasePort)) {
            $SuggCp = (Find-NextFreePort 8091).ToString()
            Write-Host "  [!] Port $CokePhasePort is IN USE. Suggested free port: $SuggCp" -ForegroundColor Yellow
        }
        $InCp = Read-Host "  [4/5] CokePhase HTML5 Port [default: $SuggCp]"
        if ($InCp) { $CokePhasePort = $InCp } else { $CokePhasePort = $SuggCp }

        # 5. CSMusicMixer Port
        $SuggMix = $MusicMixerPort
        if (Test-PortInUse ([int]$MusicMixerPort)) {
            $SuggMix = (Find-NextFreePort 8092).ToString()
            Write-Host "  [!] Port $MusicMixerPort is IN USE. Suggested free port: $SuggMix" -ForegroundColor Yellow
        }
        $InMix = Read-Host "  [5/5] CSMusicMixer Standalone Port [default: $SuggMix]"
        if ($InMix) { $MusicMixerPort = $InMix } else { $MusicMixerPort = $SuggMix }

        Write-Host "--------------------------------------------------------------------`n" -ForegroundColor Gray
    } else {
        # Non-interactive fallback: auto-shift if in use
        if (Test-PortInUse ([int]$BackendPort)) {
            $BackendPort = (Find-NextFreePort 8085).ToString()
            Write-Host "[*] Non-interactive: Port 8080 in use, auto-assigned backend to $BackendPort" -ForegroundColor Yellow
        }
        if (Test-PortInUse ([int]$DbPort)) {
            $DbPort = (Find-NextFreePort 3307).ToString()
            Write-Host "[*] Non-interactive: Port 3306 in use, auto-assigned MariaDB to $DbPort" -ForegroundColor Yellow
        }
    }
}

# Save updated configuration to .env
@"
PORT=$Port
HOST=0.0.0.0
BACKEND_PORT=$BackendPort
DB_PORT=$DbPort
COKEPHASE_PORT=$CokePhasePort
MUSICMIXER_PORT=$MusicMixerPort
DB_NAME=coke_studios
DB_USER=coke
DB_PASS=coke_secret
DB_ROOT_PASSWORD=coke_root_secret
DATABASE_URL=coke:coke_secret@tcp(mariadb:3306)/coke_studios?parseTime=true
"@ | Out-File -FilePath ".env" -Encoding utf8

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
    Write-Host "  Direct Game Client:         http://127.0.0.1:$Port/game/" -ForegroundColor White
    Write-Host "  Health Status:              http://127.0.0.1:$Port/health" -ForegroundColor White
    Write-Host "====================================================================" -ForegroundColor Green
    Write-Host "  To update at any time with zero data loss, run:" -ForegroundColor Cyan
    Write-Host "  .\setup.ps1 -Update" -ForegroundColor Yellow
    Write-Host "====================================================================" -ForegroundColor Green
} else {
    Write-Host "[*] Containers are initializing in the background. Check logs with 'docker compose logs -f'." -ForegroundColor Yellow
    Write-Host "    Then visit: http://127.0.0.1:$Port/" -ForegroundColor White
}

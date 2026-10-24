$ErrorActionPreference = 'Stop'
$rootDir = $PSScriptRoot
$runtimeDir = Join-Path $rootDir '.runtime'
New-Item -ItemType Directory -Path $runtimeDir -Force | Out-Null

$serverExe = Join-Path $rootDir 'tools\bin\coke-server.exe'
if (-not (Test-Path $serverExe)) {
    Write-Host "coke-server.exe not found. Building..."
    & (Join-Path $rootDir 'tools\build-go.ps1')
}

$port = if ($env:PORT) { $env:PORT } else { "24902" }
$webRoot = Join-Path $rootDir 'sources\gocoke\web'

$running = $false
try {
    $h = Invoke-RestMethod "http://127.0.0.1:$port/health" -TimeoutSec 1
    if ($h.project -eq 'gocoke') { $running = $true }
} catch {}

if (!$running) {
    $stdout = Join-Path $runtimeDir 'server.log'
    $stderr = Join-Path $runtimeDir 'server-error.log'
    $serverProcess = Start-Process -FilePath $serverExe -ArgumentList "-addr 0.0.0.0:$port -root `"$webRoot`"" -WorkingDirectory $rootDir -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
    $serverProcess.Id | Set-Content -LiteralPath (Join-Path $runtimeDir 'server.pid')

    $healthy = $false
    for ($attempt = 0; $attempt -lt 20; $attempt++) {
        Start-Sleep -Milliseconds 250
        if ($serverProcess.HasExited) {
            throw (Get-Content -LiteralPath $stderr -Raw)
        }
        try {
            $h = Invoke-RestMethod "http://127.0.0.1:$port/health" -TimeoutSec 1
            if ($h.project -eq 'gocoke') {
                $healthy = $true
                break
            }
        } catch {}
    }
    if (!$healthy) { throw "Go server did not become ready. See .runtime\server-error.log." }
}

Write-Host '====================================================='
Write-Host 'Coke Music / Coke Studios - Authoritative Go Server'
Write-Host '====================================================='
Write-Host "Portal & Launcher:     http://127.0.0.1:$port/"
Write-Host "Go WebAssembly Game:   http://127.0.0.1:$port/game/"
Write-Host "Server Health Status:  http://127.0.0.1:$port/health"
Write-Host '====================================================='
Write-Host 'Use Stop-Local.cmd (or Stop-Local.ps1) to stop server.'

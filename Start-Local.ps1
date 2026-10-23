$ErrorActionPreference = 'Stop'
$rootDir = $PSScriptRoot
$nodeCommand = Get-Command node.exe -ErrorAction SilentlyContinue
$nodeExe = if ($nodeCommand) { $nodeCommand.Source } else { 'C:\Program Files\nodejs\node.exe' }
if (!(Test-Path -LiteralPath $nodeExe)) { throw 'Node.js is required.' }
$runtimeDir = Join-Path $rootDir '.runtime'
New-Item -ItemType Directory -Path $runtimeDir -Force | Out-Null
$running = $false
try {
  $a = Invoke-RestMethod 'http://127.0.0.1:8080/health' -TimeoutSec 2
  $b = Invoke-RestMethod 'http://127.0.0.1:8081/health' -TimeoutSec 2
  $running = $a.project -eq 'gocoke' -and $b.project -eq 'cokephase'
} catch {}
if (!$running) {
  $serverPath = Join-Path $rootDir 'tools\serve-local.cjs'
  $serverProcess = Start-Process -FilePath $nodeExe -ArgumentList ('"' + $serverPath + '"') -WorkingDirectory $rootDir -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $runtimeDir 'server.log') -RedirectStandardError (Join-Path $runtimeDir 'server-error.log')
  $serverProcess.Id | Set-Content -LiteralPath (Join-Path $runtimeDir 'server.pid')
  $healthy = $false
  for ($attempt = 0; $attempt -lt 20; $attempt++) {
    Start-Sleep -Milliseconds 250
    if ($serverProcess.HasExited) { throw (Get-Content -LiteralPath (Join-Path $runtimeDir 'server-error.log') -Raw) }
    try {
      $a = Invoke-RestMethod 'http://127.0.0.1:8080/health' -TimeoutSec 1
      $b = Invoke-RestMethod 'http://127.0.0.1:8081/health' -TimeoutSec 1
      if ($a.project -eq 'gocoke' -and $b.project -eq 'cokephase') { $healthy = $true; break }
    } catch {}
  }
  if (!$healthy) { throw 'Local server did not become ready. See .runtime\server-error.log.' }
}
Write-Host '====================================================='
Write-Host 'Coke Music / Coke Studios Recovery Environment'
Write-Host '====================================================='
Write-Host 'Master Portal & Launcher:      http://127.0.0.1:24902/'
Write-Host 'GoCoke Client (Primary):       http://127.0.0.1:24902/game/ (or http://127.0.0.1:8080/)'
Write-Host 'CokePhase Local Demo:          http://127.0.0.1:8081/'
Write-Host 'Music Mixer Standalone:        http://127.0.0.1:8082/'
Write-Host '====================================================='
Write-Host 'Use Stop-Local.cmd (or Stop-Local.ps1) to stop servers.'

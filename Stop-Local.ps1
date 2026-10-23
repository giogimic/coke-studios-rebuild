$ErrorActionPreference = 'Stop'
$pidFile = Join-Path $PSScriptRoot '.runtime\server.pid'
if (!(Test-Path -LiteralPath $pidFile)) { Write-Host 'No recorded local server.'; exit }
$serverPid = [int](Get-Content -LiteralPath $pidFile)
$serverProcess = Get-CimInstance Win32_Process -Filter "ProcessId = $serverPid" -ErrorAction SilentlyContinue
$expectedScript = Join-Path $PSScriptRoot 'tools\serve-local.cjs'
if ($serverProcess -and $serverProcess.Name -eq 'node.exe' -and $serverProcess.CommandLine.Contains($expectedScript)) {
  Stop-Process -Id $serverPid
  Write-Host 'Local servers stopped.'
} elseif ($serverProcess) { throw 'Recorded PID belongs to another process; it was not stopped.' }
Remove-Item -LiteralPath $pidFile

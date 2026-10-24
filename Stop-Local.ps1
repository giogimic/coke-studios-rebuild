$ErrorActionPreference = 'Stop'
$pidFile = Join-Path $PSScriptRoot '.runtime\server.pid'
if (!(Test-Path -LiteralPath $pidFile)) {
    $proc = Get-Process -Name "coke-server" -ErrorAction SilentlyContinue
    if ($proc) {
        $proc | Stop-Process -Force
        Write-Host "Stopped active coke-server processes."
    } else {
        Write-Host "No recorded local server."
    }
    exit
}
$serverPid = [int](Get-Content -LiteralPath $pidFile)
$proc = Get-Process -Id $serverPid -ErrorAction SilentlyContinue
if ($proc) {
    Stop-Process -Id $serverPid -Force
    Write-Host "Go server stopped (PID $serverPid)."
}
Remove-Item -Force -LiteralPath $pidFile -ErrorAction SilentlyContinue

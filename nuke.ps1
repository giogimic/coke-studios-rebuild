# ==============================================================================
# Coke Studios 1:1 Recreation - Total Nuke & Resetup (Windows Compatibility)
# ==============================================================================
param(
    [switch]$Force,
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$RemainingArgs
)
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
& (Join-Path $ScriptDir "setup.ps1") -Nuke -Force:$Force @RemainingArgs

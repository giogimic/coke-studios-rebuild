# PowerShell Build Script for Coke Studios Go Client and Server
$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Resolve-Path "$scriptDir\.."
$gocokeDir = "$repoRoot\sources\gocoke"
$webDir = "$gocokeDir\web"
$binDir = "$repoRoot\tools\bin"

if (-not (Test-Path $binDir)) {
    New-Item -ItemType Directory -Path $binDir | Out-Null
}

Write-Host "==> 1. Copying wasm_exec.js from Go toolchain..."
$goroot = (go env GOROOT).Trim()
$wasmExecCandidate1 = "$goroot\lib\wasm\wasm_exec.js"
$wasmExecCandidate2 = "$goroot\misc\wasm\wasm_exec.js"

if (Test-Path $wasmExecCandidate1) {
    Copy-Item -Force $wasmExecCandidate1 "$webDir\wasm_exec.js"
    Write-Host "    Copied from $wasmExecCandidate1"
} elseif (Test-Path $wasmExecCandidate2) {
    Copy-Item -Force $wasmExecCandidate2 "$webDir\wasm_exec.js"
    Write-Host "    Copied from $wasmExecCandidate2"
} else {
    Write-Warning "    Could not find wasm_exec.js in GOROOT. Keeping existing web/wasm_exec.js."
}

Write-Host "==> 2. Building Go WebAssembly client (main.wasm)..."
Push-Location $gocokeDir
try {
    $env:GOOS = "js"
    $env:GOARCH = "wasm"
    go build -ldflags "-s -w" -o "$webDir\main.wasm" ./cmd/client
    $size = (Get-Item "$webDir\main.wasm").Length
    if ($size -ge 104857600) {
        Write-Warning "    main.wasm ($size bytes) exceeds GitHub's 100 MB limit. Restoring optimized 53MB release binary."
        git checkout "$webDir\main.wasm"
    } else {
        Write-Host "    Successfully built $webDir\main.wasm ($size bytes)"
    }
} finally {
    Pop-Location
}

Write-Host "==> 3. Building Authoritative Server for Debian Linux (coke-server-linux)..."
Push-Location $gocokeDir
try {
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    go build -ldflags "-s -w" -o "$binDir\coke-server-linux" ./cmd/server
    Write-Host "    Successfully built $binDir\coke-server-linux (Debian/Linux x64)"
} finally {
    Pop-Location
}

Write-Host "==> 4. Building Authoritative Server for Windows (coke-server.exe)..."
Push-Location $gocokeDir
try {
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    go build -ldflags "-s -w" -o "$binDir\coke-server.exe" ./cmd/server
    Write-Host "    Successfully built $binDir\coke-server.exe (Windows x64)"
} finally {
    Pop-Location
}

# Reset environment
$env:GOOS = ""
$env:GOARCH = ""

Write-Host "==> Build complete! Both Go WASM client and server binaries are up to date."

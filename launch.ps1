# launch.ps1 — build and open server + two clients in separate windows.
#
# Usage:
#   .\launch.ps1              # normal run
#   .\launch.ps1 -Profile     # server exposes pprof on :6060
#
# Profiling quick-start (after -Profile):
#   go tool pprof http://localhost:6060/debug/pprof/goroutine
#   go tool pprof http://localhost:6060/debug/pprof/heap
#   go tool pprof "http://localhost:6060/debug/pprof/profile?seconds=10"

param([switch]$Profile)

$ErrorActionPreference = "Stop"

Write-Host "Building..." -ForegroundColor Cyan
go build -o chat.exe .
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Host "Build OK" -ForegroundColor Green

$serverCmd = ".\chat.exe -mode=server"
if ($Profile) {
    $serverCmd += " -pprof=:6060"
    Write-Host "pprof: http://localhost:6060/debug/pprof/" -ForegroundColor Yellow
}

# Server window
Start-Process powershell -ArgumentList "-NoExit", "-Command", $serverCmd

# Give the server a moment to bind the port before clients connect.
Start-Sleep -Milliseconds 400

# Two client windows
Start-Process powershell -ArgumentList "-NoExit", "-Command", ".\chat.exe -mode=client"
Start-Process powershell -ArgumentList "-NoExit", "-Command", ".\chat.exe -mode=client"

Write-Host "Launched: 1 server + 2 clients" -ForegroundColor Green

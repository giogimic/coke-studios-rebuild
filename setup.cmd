@echo off
setlocal
set "ARGS=%*"
if "%~1"=="--" if /I "%~2"=="update" (
    set "ARGS=-Update %3 %4 %5 %6 %7 %8 %9"
)
if /I "%~1"=="--update" (
    set "ARGS=-Update %2 %3 %4 %5 %6 %7 %8 %9"
)
if /I "%~1"=="update" (
    set "ARGS=-Update %2 %3 %4 %5 %6 %7 %8 %9"
)
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0setup.ps1" %ARGS%

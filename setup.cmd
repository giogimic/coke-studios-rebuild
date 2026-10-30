@echo off
setlocal
set "ARGS=%*"
if "%~1"=="--" if /I "%~2"=="update" set "ARGS=-Update"
if /I "%~1"=="--update" set "ARGS=-Update"
if /I "%~1"=="update" set "ARGS=-Update"
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0setup.ps1" %ARGS%

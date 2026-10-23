@echo off
echo GoCoke offline client preview: http://127.0.0.1:8080/
echo This static preview does not provide a multiplayer game backend.
"%~dp0tools\bin\gocoke-static.exe" -addr 127.0.0.1:8080 -root "%~dp0sources\gocoke\web"
pause

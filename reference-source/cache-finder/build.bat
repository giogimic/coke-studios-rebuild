@echo off
rem Builds build\CokeCacheFinder.exe: one .exe for every Windows from 95 to 11.
rem Needs Visual Studio (any edition) with the C++ tools.
setlocal
cd /d "%~dp0"

set "VSWHERE=%ProgramFiles(x86)%\Microsoft Visual Studio\Installer\vswhere.exe"
if not exist "%VSWHERE%" (
    echo Visual Studio was not found.
    exit /b 1
)
for /f "usebackq delims=" %%i in (`"%VSWHERE%" -latest -products * -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath`) do set "VS=%%i"
if not defined VS (
    echo Visual Studio's C++ tools were not found.
    exit /b 1
)
call "%VS%\VC\Auxiliary\Build\vcvarsall.bat" x86 >nul 2>&1 || exit /b 1

if not exist build mkdir build
del /q build\*.obj 2>nul

rem /arch:IA32  no SSE: Pentium II and Athlon XP-era CPUs do not have SSE2
rem /GS- /Zl    no security cookie and no default runtime library: there is no C runtime
cl /nologo /c /O1 /GS- /Zl /arch:IA32 /W3 /wd4996 /utf-8 /Fobuild\ src\*.c || exit /b 1

link /nologo /NODEFAULTLIB /ENTRY:ccf_entry /SUBSYSTEM:CONSOLE,5.01 /OPT:REF /OPT:ICF ^
     /MANIFEST:EMBED /MANIFESTUAC:"level='asInvoker' uiAccess='false'" /MANIFESTINPUT:src\app.manifest ^
     /OUT:build\CokeCacheFinder.exe build\*.obj kernel32.lib || exit /b 1

rem link.exe will not write a version below 5.01 (XP); the loader of 95, 98, ME
rem and NT 4 needs 4.0, so the header is patched afterwards.
powershell -NoProfile -ExecutionPolicy Bypass -File tools\pe-version.ps1 build\CokeCacheFinder.exe || exit /b 1

echo Built build\CokeCacheFinder.exe

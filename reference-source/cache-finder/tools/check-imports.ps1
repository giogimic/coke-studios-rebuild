# Fails if CokeCacheFinder.exe imports anything Windows 95's kernel32 lacks,
# or anything from another DLL. Newer functions must be looked up at run time
# with GetProcAddress (see plat_win32.c), or the .exe stops starting on old PCs.
param([Parameter(Mandatory = $true)][string]$Path)

$allowed = @(
    'CloseHandle', 'CreateDirectoryA', 'CreateFileA', 'CreateProcessA', 'DeviceIoControl', 'ExitProcess',
    'FileTimeToSystemTime', 'FindClose', 'FindFirstFileA', 'FindNextFileA', 'GetCommandLineA', 'GetConsoleMode',
    'GetCurrentProcessId', 'GetDriveTypeA', 'GetFileAttributesA', 'GetFileSize', 'GetLastError', 'GetLocalTime',
    'GetLogicalDrives', 'GetModuleFileNameA', 'GetModuleHandleA', 'GetProcAddress', 'GetProcessHeap',
    'GetStdHandle', 'GetTickCount', 'GetVersion', 'GetVersionExA', 'GetVolumeInformationA',
    'GetWindowsDirectoryA', 'HeapAlloc', 'HeapFree', 'HeapReAlloc', 'LoadLibraryA', 'MultiByteToWideChar',
    'ReadConsoleA', 'ReadFile', 'SetConsoleCtrlHandler', 'SetErrorMode', 'SetFilePointer', 'VirtualAlloc',
    'VirtualFree', 'WideCharToMultiByte', 'WriteFile'
)

$vswhere = "${env:ProgramFiles(x86)}\Microsoft Visual Studio\Installer\vswhere.exe"
$vs = & $vswhere -latest -products * -property installationPath
$dumpbin = Get-ChildItem "$vs\VC\Tools\MSVC\*\bin\Hostx64\x86\dumpbin.exe" | Select-Object -First 1
$out = & $dumpbin.FullName /nologo /imports $Path

$dlls = $out | Where-Object { $_ -match '^\s{4}\S+\.dll$' } | ForEach-Object { $_.Trim().ToLower() }
$funcs = $out | Where-Object { $_ -match '^\s{8,}[0-9A-F]+\s+\w+$' } | ForEach-Object { ($_ -split '\s+')[-1] }

$bad = @($dlls | Where-Object { $_ -ne 'kernel32.dll' }) + @($funcs | Where-Object { $allowed -notcontains $_ })
if ($bad.Count) {
    Write-Error ("Not available on Windows 95: " + ($bad -join ', '))
    exit 1
}
"Imports OK: kernel32.dll, $($funcs.Count) functions, all present in Windows 95."

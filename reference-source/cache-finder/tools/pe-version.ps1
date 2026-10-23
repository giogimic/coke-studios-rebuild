# Marks a 32-bit .exe as needing only Windows 4.0 (95 / NT 4), which modern
# linkers refuse to write. Windows 95, 98 and ME will not start an .exe whose
# header asks for a newer Windows, even when nothing in it needs one.
param([Parameter(Mandatory = $true)][string]$Path)

$bytes = [IO.File]::ReadAllBytes($Path)
$pe = [BitConverter]::ToInt32($bytes, 0x3C)
if ([Text.Encoding]::ASCII.GetString($bytes, $pe, 4) -ne "PE`0`0") { throw "$Path is not a PE file" }
$opt = $pe + 24
if ([BitConverter]::ToUInt16($bytes, $opt) -ne 0x10B) { throw "$Path is not a 32-bit image" }

foreach ($field in 40, 48) {           # MajorOperatingSystemVersion, MajorSubsystemVersion
    $bytes[$opt + $field] = 4; $bytes[$opt + $field + 1] = 0
    $bytes[$opt + $field + 2] = 0; $bytes[$opt + $field + 3] = 0   # and the minor version
}
[IO.File]::WriteAllBytes($Path, $bytes)

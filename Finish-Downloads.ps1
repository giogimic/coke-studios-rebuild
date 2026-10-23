$ErrorActionPreference = 'Stop'
$taskRoot = [System.IO.Path]::GetFullPath($PSScriptRoot)
$manifestFile = Join-Path $taskRoot 'docs\SOURCE-MANIFEST.json'
$manifest = Get-Content -LiteralPath $manifestFile -Raw | ConvertFrom-Json
$gitCommand = Get-Command git.exe -ErrorAction SilentlyContinue
$gitExe = if ($gitCommand) { $gitCommand.Source } else { 'C:\Program Files\Git\cmd\git.exe' }
if (!(Test-Path -LiteralPath $gitExe)) { throw 'Git for Windows is required.' }
$results = @()
foreach ($source in $manifest.sources) {
  $dest = [System.IO.Path]::GetFullPath((Join-Path $taskRoot ('sources\' + $source.folder)))
  if (!$dest.StartsWith($taskRoot + '\', [StringComparison]::OrdinalIgnoreCase)) { throw 'Invalid destination in manifest.' }
  if (Test-Path -LiteralPath $dest) {
    if (Test-Path -LiteralPath (Join-Path $dest '.git')) {
      & $gitExe -C $dest rev-parse --verify HEAD 2>$null | Out-Null
      if ($LASTEXITCODE -eq 0) {
        $commit = (& $gitExe -C $dest rev-parse HEAD).Trim()
        $results += [pscustomobject]@{repo=$source.repo;status='present';commit=$commit}
        Write-Host ('Already present: ' + $source.repo)
        continue
      }
    }
    $results += [pscustomobject]@{repo=$source.repo;status='existing-incomplete-directory';commit=$null}
    Write-Warning ('Existing directory needs inspection: ' + $dest)
    continue
  }
  Write-Host ('Downloading ' + $source.repo)
  & $gitExe -c credential.helper= clone --branch $source.branch ('https://github.com/' + $source.repo + '.git') $dest
  if ($LASTEXITCODE -ne 0) {
    $results += [pscustomobject]@{repo=$source.repo;status='download-failed';commit=$null}
    continue
  }
  if ($source.pin) {
    & $gitExe -C $dest checkout --detach $source.pin
    if ($LASTEXITCODE -ne 0) {
      $results += [pscustomobject]@{repo=$source.repo;status='pin-failed';commit=$null}
      continue
    }
  }
  $commit = (& $gitExe -C $dest rev-parse HEAD).Trim()
  $results += [pscustomobject]@{repo=$source.repo;status='downloaded';commit=$commit}
}
$archiveDir = Join-Path $taskRoot 'archives'
New-Item -ItemType Directory -Path $archiveDir -Force | Out-Null
$archive = Join-Path $archiveDir 'CokeStudios-The-Complete-Pack.rar'
$archiveUrl = 'https://h4bbo.net/archive/HabboStuff/CokeStudios%20The%20Complete%20Pack.rar'
try {
  if (!(Test-Path -LiteralPath $archive)) {
    Write-Host 'Downloading the legacy Complete Pack archive (about 80 MB).'
    $partial = $archive + '.part'
    Invoke-WebRequest -Uri $archiveUrl -OutFile $partial -UseBasicParsing -TimeoutSec 600
    $stream = [System.IO.File]::OpenRead($partial)
    try { $header = New-Object byte[] 4; $readCount = $stream.Read($header,0,4) } finally { $stream.Dispose() }
    if ($readCount -ne 4 -or [Text.Encoding]::ASCII.GetString($header) -ne 'Rar!') { throw 'Download did not have a RAR signature; retained as .part for inspection.' }
    # Both source and destination are explicit files inside this task's archives folder.
    Move-Item -LiteralPath $partial -Destination $archive
  }
  $hash = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash
  $results += [pscustomobject]@{repo=$archiveUrl;status='archive-present';commit=$hash}
} catch {
  Write-Warning $_.Exception.Message
  $results += [pscustomobject]@{repo=$archiveUrl;status='archive-download-failed';commit=$null}
}
$results | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $taskRoot 'docs\DOWNLOAD-RESULTS.json') -Encoding UTF8
$failed = @($results | Where-Object { $_.status -match 'failed|incomplete' })
Write-Host ('Finished: ' + ($results.Count - $failed.Count) + ' available; ' + $failed.Count + ' still pending.')
Write-Host 'Legacy archives are preserved only; this script does not run their installers or servers.'
if ($failed.Count -gt 0) { exit 1 }

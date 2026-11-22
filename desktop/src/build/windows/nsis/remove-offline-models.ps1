param()
$ErrorActionPreference = 'Stop'
try {
  $modelHome = if ([string]::IsNullOrWhiteSpace($env:OCTOP_HOME)) { Join-Path $env:USERPROFILE '.octop-demo' } else { [Environment]::ExpandEnvironmentVariables($env:OCTOP_HOME) }
  if (-not [IO.Path]::IsPathRooted($modelHome)) { throw 'OCTOP_HOME must be absolute' }
  $root = [IO.Path]::GetFullPath($modelHome)
  if ($root.TrimEnd('\') -eq [IO.Path]::GetPathRoot($root).TrimEnd('\')) { throw 'Unsafe root' }
  $current = $root
  foreach ($part in @('') + @('speech-models','sensevoice-int8')) {
    if ($part -ne '') { $current = Join-Path $current $part }
    if (-not (Test-Path -LiteralPath $current)) { exit 0 }
    $entry = Get-Item -LiteralPath $current -Force
    if (-not $entry.PSIsContainer -or ($entry.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Unsafe model path' }
  }
  foreach ($entry in @(Get-ChildItem -LiteralPath $current -Force)) {
    if ($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) { $entry.Delete() }
    else { Remove-Item -LiteralPath $entry.FullName -Force -Recurse }
  }
  Remove-Item -LiteralPath $current -Force
  exit 0
} catch { Write-Error $_.Exception.Message; exit 1 }

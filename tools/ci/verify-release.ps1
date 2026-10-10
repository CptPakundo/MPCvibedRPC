# Checks a published release the way a user gets it: the ten files, each program's checksum and build record, the
# exe's version details, that it is the latest release, and a real self-update from the release before it (in a data
# folder of its own, with no window and presence off). Runs on GitHub after the release workflow has published.
param([Parameter(Mandatory)][string]$Version)
$ErrorActionPreference = 'Stop'
$repo = $env:GITHUB_REPOSITORY
$tag = "v$Version"
$work = Join-Path $env:RUNNER_TEMP ('verify-' + [guid]::NewGuid().ToString('N').Substring(0, 8))
New-Item -ItemType Directory -Path $work | Out-Null
$failures = New-Object System.Collections.ArrayList
function Check([bool]$ok, [string]$what) {
  if ($ok) { Write-Host "PASS  $what" } else { Write-Host "FAIL  $what"; [void]$failures.Add($what) }
}
# only the copies started from this check's folder (an installed program is never touched)
function Ours { Get-Process -Name 'MPCvibedRPC' -ErrorAction SilentlyContinue | Where-Object { $_.Path -and $_.Path.StartsWith($work) } }
function WaitFor([scriptblock]$cond, [int]$seconds, [string]$what) {
  $end = (Get-Date).AddSeconds($seconds)
  while ((Get-Date) -lt $end) { try { if (& $cond) { return $true } } catch {}; Start-Sleep -Milliseconds 500 }
  Write-Host "timed out: $what"; return $false
}

$programs = 'MPCvibedRPC.exe', 'MPCvibedRPC-linux-amd64', 'MPCvibedRPC-linux-arm64', 'MPCvibedRPC-macos.zip'
$rel = Join-Path $work 'release'
try {
  gh release download $tag --repo $repo --dir $rel
  $want = @($programs + ($programs | ForEach-Object { "$_.sha256" }) + 'LICENSE', 'THIRD-PARTY-NOTICES.md') | Sort-Object
  $got = @(Get-ChildItem $rel | ForEach-Object Name) | Sort-Object
  Check (($want -join ',') -eq ($got -join ',')) "the release has its $($want.Count) files ($($got -join ', '))"
  foreach ($f in $programs) {
    $path = Join-Path $rel $f
    $line = (Get-Content "$path.sha256" -Raw).Trim()
    $hash = (Get-FileHash $path -Algorithm SHA256).Hash.ToLower()
    Check ($line -eq "$hash  $f") "$f matches its checksum"
    gh attestation verify $path --repo $repo | Out-Host
    Check ($LASTEXITCODE -eq 0) "$f has a valid build record from this repository"
  }
  $vi = (Get-Item (Join-Path $rel 'MPCvibedRPC.exe')).VersionInfo
  Check ($vi.ProductVersion -eq $Version -and $vi.FileVersion -like "$Version*") "the exe's version details say $Version ($($vi.FileVersion))"
  $latest = gh release view --repo $repo --json tagName --jq .tagName
  Check ($latest -eq $tag) "it is the latest release ($latest)"

  # a real self-update: the previous release's exe finds this one on GitHub, downloads it, checks it and restarts as it
  $prev = gh release list --repo $repo --exclude-drafts --exclude-pre-releases --limit 20 --json tagName --jq '.[].tagName' | Where-Object { $_ -ne $tag } | Select-Object -First 1
  Write-Host "previous release: $prev"
  $dir = Join-Path $work 'update'
  gh release download $prev --repo $repo --pattern 'MPCvibedRPC.exe' --dir $dir
  $app = Join-Path $dir 'MPCvibedRPC.exe'
  $home_ = Join-Path $work 'home'; New-Item -ItemType Directory -Path $home_ | Out-Null
  Set-Content (Join-Path $home_ 'config.json') '{"app":{"openWindow":false,"startPresence":false,"checkUpdates":false,"welcomeSeen":true}}'
  $env:MPCRPC_HOME = $home_
  $env:MPCRPC_BROWSER = 'none'
  function Ipc { $f = Join-Path $home_ 'ipc.json'; if (Test-Path $f) { try { return Get-Content $f -Raw | ConvertFrom-Json } catch {} }; return $null }
  function Api([string]$name) {
    $i = Ipc
    Invoke-RestMethod -Uri "http://127.0.0.1:$($i.port)/api/$name" -Method Post -Headers @{ 'X-Token' = $i.token } -Body '{}' -ContentType 'application/json' -TimeoutSec 120
  }
  function Ping { $j = Ipc; if ($null -eq $j) { return $null }; (Invoke-RestMethod "http://127.0.0.1:$($j.port)/api/ping" -TimeoutSec 2).version }
  $p = Start-Process $app -ArgumentList '--background' -PassThru
  Check (WaitFor { (Ping) -eq $prev.TrimStart('v') } 30 'old version up') "$prev starts"
  $u = Api 'update-check'
  Check ($u.newer -eq $true -and $u.latest -eq $Version) "$prev is offered $Version (offered $($u.latest))"
  Api 'update-install' | Out-Null
  Check ($p.WaitForExit(60000)) "$prev exits to update"
  Check (WaitFor { (Ping) -eq $Version } 60 'new version up') "$Version runs after the update"
  Check ((Get-FileHash $app).Hash -eq (Get-FileHash (Join-Path $rel 'MPCvibedRPC.exe')).Hash) 'the updated file is the released exe'
  try { Api 'quit' | Out-Null } catch {}
  Check (WaitFor { -not (Ours) } 20 'quit') 'the updated program quits'
  Write-Host '--- log of the updated copy ---'
  Get-Content (Join-Path $home_ 'mpcvibedrpc.log') -ErrorAction SilentlyContinue | Write-Host
}
catch { Write-Host "ERROR $_"; [void]$failures.Add("exception: $_") }
finally { Ours | Stop-Process -Force }
if ($failures.Count) {
  Write-Host "::error title=Release check failed::$($failures -join '; ')"
  Write-Host "`n$($failures.Count) check(s) failed:"; $failures | ForEach-Object { Write-Host " - $_" }; exit 1
}
Write-Host "`nThe published release checks out."

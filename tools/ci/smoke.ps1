# Smoke test of the built Windows program: starts it for real (hidden, with its own data folder), talks to it through
# its local API and the tray icon's window, and plays a fake MPC-HC into a fake Discord.
param(
  [Parameter(Mandatory)][string]$Exe,       # the build under test (version 0.9.2)
  [Parameter(Mandatory)][string]$NewExe,    # the same program built with the next patch version, offered as an update
  [string]$Version = '0.9.2',
  [string]$NextVersion = '0.9.3'
)
$ErrorActionPreference = 'Stop'
$work = Join-Path $env:RUNNER_TEMP ('smoke-' + [guid]::NewGuid().ToString('N').Substring(0, 8))
New-Item -ItemType Directory -Path $work | Out-Null
$home_ = Join-Path $work 'home'; New-Item -ItemType Directory -Path $home_ | Out-Null
$app = Join-Path $work 'MPCvibedRPC.exe'
Copy-Item $Exe $app
$env:MPCRPC_HOME = $home_
# A copy with its own data folder gets its own run-at-login value (see winsys/autostart.go), so this test can never touch an installed program's.
$runName = 'MPCvibedRPC-' + ([BitConverter]::ToString([Security.Cryptography.SHA256]::Create().ComputeHash([Text.Encoding]::UTF8.GetBytes($home_.TrimEnd('\','/').ToLower()))).Replace('-','').Substring(0,8).ToLower())
$port = 13591
$env:MPCRPC_UPDATE_API = "http://127.0.0.1:$port"
$stateFile = Join-Path $work 'state.txt'; Set-Content $stateFile '2'
$discordLog = Join-Path $work 'discord.log'
$failures = New-Object System.Collections.ArrayList
$procs = New-Object System.Collections.ArrayList

function Check([bool]$ok, [string]$what) {
  if ($ok) { Write-Host "PASS  $what" } else { Write-Host "FAIL  $what"; [void]$failures.Add($what) }
}
function WaitFor([scriptblock]$cond, [int]$seconds, [string]$what) {
  $end = (Get-Date).AddSeconds($seconds)
  while ((Get-Date) -lt $end) { try { if (& $cond) { return $true } } catch {}; Start-Sleep -Milliseconds 200 }
  Write-Host "timed out: $what"; return $false
}
function Ipc { $f = Join-Path $home_ 'ipc.json'; if (Test-Path $f) { try { return Get-Content $f -Raw | ConvertFrom-Json } catch {} }; return $null }
function Api([string]$method, [string]$name, $body = $null) {
  $i = Ipc
  $args_ = @{ Uri = "http://127.0.0.1:$($i.port)/api/$name"; Method = $method; Headers = @{ 'X-Token' = $i.token }; TimeoutSec = 30 }
  if ($method -eq 'Post') { $args_.Body = if ($null -eq $body) { '{}' } else { $body | ConvertTo-Json -Depth 6 }; $args_.ContentType = 'application/json' }
  Invoke-RestMethod @args_
}
function Dump { Write-Host '--- app log ---'; Get-Content (Join-Path $home_ 'mpcvibedrpc.log') -ErrorAction SilentlyContinue | Write-Host; Write-Host '--- fake discord log ---'; Get-Content $discordLog -ErrorAction SilentlyContinue | Write-Host }

Add-Type -Namespace W -Name U -MemberDefinition @'
[System.Runtime.InteropServices.DllImport("user32.dll", CharSet = System.Runtime.InteropServices.CharSet.Unicode)] public static extern System.IntPtr FindWindow(string c, string t);
[System.Runtime.InteropServices.DllImport("user32.dll")] public static extern bool PostMessage(System.IntPtr h, uint m, System.UIntPtr w, System.IntPtr l);
'@

try {
  # fakes
  [void]$procs.Add((Start-Process pwsh -PassThru -WindowStyle Hidden -ArgumentList '-NoProfile', '-File', (Join-Path $PSScriptRoot 'fake-discord.ps1'), '-Log', $discordLog))
  [void]$procs.Add((Start-Process pwsh -PassThru -WindowStyle Hidden -ArgumentList '-NoProfile', '-File', (Join-Path $PSScriptRoot 'fake-http.ps1'), '-Port', $port, '-StateFile', $stateFile, '-NewExe', $NewExe, '-Tag', $NextVersion))
  Check (WaitFor { (Test-Path $discordLog) -and ((Get-Content $discordLog -Raw) -match 'LISTENING') } 20 'fake discord up') 'fake Discord pipe is listening'
  Check (WaitFor { (Invoke-WebRequest "http://127.0.0.1:$port/variables.html" -UseBasicParsing -TimeoutSec 2).StatusCode -eq 200 } 20 'fake http up') 'fake MPC-HC web interface is up'

  # settings: point the program at the fake MPC-HC; no artwork lookups from CI
  Set-Content (Join-Path $home_ 'config.json') (@{ port = $port; pollInterval = 500; showArtwork = $false } | ConvertTo-Json)

  # the program carries its version details and icon
  $vi = (Get-Item $app).VersionInfo
  Check ($vi.ProductVersion -eq $Version -and $vi.ProductName -eq 'MPCvibedRPC' -and $vi.FileDescription -like 'MPCvibedRPC*') "exe carries version details ($($vi.ProductVersion))"
  Add-Type -AssemblyName System.Drawing
  $ico = [System.Drawing.Icon]::ExtractAssociatedIcon($app)
  $bmp = $ico.ToBitmap(); $px = @(); foreach ($pt in @(@(8, 8), @(16, 16), @(24, 24), @(8, 24))) { $px += $bmp.GetPixel($pt[0], $pt[1]).ToArgb() }
  Write-Host "icon pixels: $($px -join ',') size $($bmp.Width)x$($bmp.Height)"
  Check (($px | Select-Object -Unique).Count -gt 1) 'exe has a non-blank icon'

  # start in the background, like the registry entry does
  $p = Start-Process $app -ArgumentList '--background' -PassThru
  [void]$procs.Add($p)
  Check (WaitFor { $null -ne (Ipc) } 20 'ipc.json') 'program started and published ipc.json'
  $i = Ipc
  $ping = Invoke-RestMethod "http://127.0.0.1:$($i.port)/api/ping"
  Check ($ping.app -eq 'MPCvibedRPC' -and $ping.version -eq $Version) 'ping answers with the version'
  $state = Api 'Get' 'state'
  Write-Host "state: canAutoStart=$($state.canAutoStart) sections=$(@($state.schema).Count) dataDir=$($state.dataDir) expected=$home_"
  Check ($state.canAutoStart -eq $true -and @($state.schema).Count -ge 3 -and (Resolve-Path $state.dataDir).Path -eq (Resolve-Path $home_).Path) 'state carries schema, canAutoStart and data folder'
  try { Invoke-RestMethod "http://127.0.0.1:$($i.port)/api/state" -TimeoutSec 5 | Out-Null; Check $false 'API without token is refused' } catch { Check ($_.Exception.Response.StatusCode.value__ -eq 401) 'API without token is refused' }

  # presence reaches "Discord" through the real named pipe
  Check (WaitFor { (Get-Content $discordLog -Raw) -match 'Sample Movie' } 30 'activity in fake discord') 'presence frame with the title arrives over the named pipe'
  $s = Api 'Get' 'status'
  Check ($s.running -and $s.discord -eq 'connected' -and $s.mpc) 'status: connected, MPC-HC seen'
  Check ((Get-Content $discordLog -Raw) -match '"type":3') 'activity uses the Watching type'

  # closing the file clears the presence
  $before = (Get-Content $discordLog).Count
  Set-Content $stateFile '0'
  Check (WaitFor { $lines = Get-Content $discordLog; $lines.Count -gt $before -and ($lines[-1] -match 'FRAME 1' ) -and ($lines[-1] -notmatch '"activity"') } 20 'clear') 'presence is cleared when playback stops'
  Set-Content $stateFile '2'

  # tray: its hidden window exists and understands the menu commands (1 open, 2 toggle, 3 quit)
  $hwnd = [W.U]::FindWindow('MPCDiscordRPC.Tray', [NullString]::Value)
  Check ($hwnd -ne [IntPtr]::Zero) 'tray window exists'
  [void][W.U]::PostMessage($hwnd, 0x0111, [UIntPtr]2, [IntPtr]::Zero)
  Check (WaitFor { -not (Api 'Get' 'status').running } 10 'toggle off') 'tray menu "Stop presence" works'
  [void][W.U]::PostMessage($hwnd, 0x0111, [UIntPtr]2, [IntPtr]::Zero)
  Check (WaitFor { (Api 'Get' 'status').running } 10 'toggle on') 'tray menu "Start presence" works'

  # run at login
  $plainBefore = (reg query 'HKCU\Software\Microsoft\Windows\CurrentVersion\Run' /v MPCvibedRPC 2>&1) -join "`n"
  Api 'Post' 'save' @{ app = @{ autoStart = $true } } | Out-Null
  $reg = (reg query 'HKCU\Software\Microsoft\Windows\CurrentVersion\Run' /v $runName 2>&1) -join "`n"
  Check ($reg -match 'MPCvibedRPC\.exe" --background') 'autostart writes the Run entry (under the name for this data folder)'
  Check ((Api 'Get' 'state').autoStart -eq $true) 'state reports autostart on'
  Api 'Post' 'save' @{ app = @{ autoStart = $false } } | Out-Null
  $reg = (reg query 'HKCU\Software\Microsoft\Windows\CurrentVersion\Run' /v $runName 2>&1) -join "`n"
  Check ($reg -notmatch 'MPCvibedRPC\.exe') 'autostart removes the Run entry'
  $plain = (reg query 'HKCU\Software\Microsoft\Windows\CurrentVersion\Run' /v MPCvibedRPC 2>&1) -join "`n"
  Check ($plain -eq $plainBefore) 'the plain (installed program) Run value was never touched'

  # MPC-HC web interface switch (MPC-HC itself is not installed on the runner: the registry part is what we can check)
  $mw = Api 'Post' 'mpc-web' @{ closeMpc = $false }
  Check ($mw.ok -eq $true) 'mpc-web reports success'
  $reg = (reg query 'HKCU\Software\MPC-HC\MPC-HC\Settings' /v EnableWebServer 2>&1) -join "`n"
  Check ($reg -match '0x1') 'mpc-web set EnableWebServer=1'
  $reg = (reg query 'HKCU\Software\MPC-HC\MPC-HC\Settings' /v WebServerPort 2>&1) -join "`n"
  Check ($reg -match ('0x' + ('{0:x}' -f $port))) 'mpc-web set WebServerPort'

  # a second launch hands over to the running copy and exits
  $second = Start-Process $app -PassThru
  Check ($second.WaitForExit(15000) -and $second.ExitCode -eq 0) 'a second launch exits quietly'

  # update: offered, downloaded, swapped, restarted as the new version
  $u = Api 'Post' 'update-check'
  Check ($u.newer -eq $true -and $u.latest -eq $NextVersion) 'update is offered'
  $connectsBefore = ([regex]::Matches((Get-Content $discordLog -Raw), '(?<!DIS)CONNECTED')).Count
  Api 'Post' 'update-install' | Out-Null
  Check ($p.WaitForExit(20000)) 'old program exits after updating'
  Check (WaitFor { $j = Ipc; if ($null -eq $j) { return $false }; (Invoke-RestMethod "http://127.0.0.1:$($j.port)/api/ping" -TimeoutSec 2).version -eq $NextVersion } 30 'new version') 'new version is running after the update'
  Check ((Get-FileHash $app).Hash -eq (Get-FileHash $NewExe).Hash) 'the program file was replaced'
  Check (WaitFor { ([regex]::Matches((Get-Content $discordLog -Raw), '(?<!DIS)CONNECTED')).Count -gt $connectsBefore } 30 'reconnected') 'new version reconnects to Discord'

  # quit from the tray menu
  $hwnd = [W.U]::FindWindow('MPCDiscordRPC.Tray', [NullString]::Value)
  [void][W.U]::PostMessage($hwnd, 0x0111, [UIntPtr]3, [IntPtr]::Zero)
  Check (WaitFor { $null -eq (Ipc) } 15 'ipc gone') 'tray menu "Quit" shuts the program down'
  Check (WaitFor { -not (Get-Process -Name 'MPCvibedRPC' -ErrorAction SilentlyContinue) } 15 'process gone') 'no program process is left'
}
catch { Write-Host "ERROR $_"; [void]$failures.Add("exception: $_") }
finally {
  Dump
  Get-Process -Name 'MPCvibedRPC' -ErrorAction SilentlyContinue | Stop-Process -Force
  foreach ($x in $procs) { try { if (-not $x.HasExited) { $x.Kill() } } catch {} }
}
function Annotate([string]$title, [string[]]$lines) {
  $t = ($lines | Select-Object -Last 120 | ForEach-Object { if ($_.Length -gt 400) { $_.Substring(0, 400) } else { $_ } }) -join "`n"
  $t = $t.Replace('%', '%25').Replace("`r", '').Replace("`n", '%0A')
  Write-Host "::error title=$title::$t"
}
if ($failures.Count) {
  $log = @(Get-Content (Join-Path $home_ 'mpcvibedrpc.log') -ErrorAction SilentlyContinue) + @('--- fake discord ---') + @(Get-Content $discordLog -ErrorAction SilentlyContinue)
  Annotate 'Smoke test failed' (@($failures | ForEach-Object { "FAILED: $_" }) + $log)
}
if ($failures.Count) { Write-Host "`n$($failures.Count) check(s) failed:"; $failures | ForEach-Object { Write-Host " - $_" }; exit 1 }
Write-Host "`nAll smoke checks passed."

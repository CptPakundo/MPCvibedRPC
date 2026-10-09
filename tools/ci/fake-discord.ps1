# A stand-in for the Discord desktop app: a named pipe \\.\pipe\discord-ipc-0 that answers the handshake and
# acknowledges every command, logging each frame it receives to $Log.
param([Parameter(Mandatory)][string]$Log)
$ErrorActionPreference = 'Stop'
function NewPipe { New-Object System.IO.Pipes.NamedPipeServerStream('discord-ipc-0', [System.IO.Pipes.PipeDirection]::InOut, 10, [System.IO.Pipes.PipeTransmissionMode]::Byte, [System.IO.Pipes.PipeOptions]::None) }

function ReadExact($s, [int]$n) {
  $b = New-Object byte[] $n; $o = 0
  while ($o -lt $n) { $r = $s.Read($b, $o, $n - $o); if ($r -le 0) { return $null }; $o += $r }
  return , $b
}
function Send($s, [int]$op, [string]$json) {
  $body = [Text.Encoding]::UTF8.GetBytes($json)
  $h = New-Object byte[] 8
  [BitConverter]::GetBytes([uint32]$op).CopyTo($h, 0)
  [BitConverter]::GetBytes([uint32]$body.Length).CopyTo($h, 4)
  $s.Write($h, 0, 8); $s.Write($body, 0, $body.Length); $s.Flush()
}

while ($true) {
  $pipe = NewPipe
  Add-Content $Log 'LISTENING'
  $pipe.WaitForConnection()
  Add-Content $Log 'CONNECTED'
  try {
    while ($true) {
      $h = ReadExact $pipe 8
      if ($null -eq $h) { break }
      $op = [BitConverter]::ToUInt32($h, 0); $len = [BitConverter]::ToUInt32($h, 4)
      $txt = ''
      if ($len -gt 0) {
        $body = ReadExact $pipe $len
        if ($null -eq $body) { break }
        $txt = [Text.Encoding]::UTF8.GetString($body)
      }
      Add-Content $Log "FRAME $op $txt"
      if ($op -eq 0) { Send $pipe 1 '{"cmd":"DISPATCH","evt":"READY","data":{"v":1}}' }
      elseif ($op -eq 1) {
        $m = $txt | ConvertFrom-Json
        Send $pipe 1 ('{"cmd":"SET_ACTIVITY","evt":null,"nonce":"' + $m.nonce + '","data":null}')
      }
    }
  } catch { Add-Content $Log "ERR $_" }
  try { $pipe.Dispose() } catch {}
  Add-Content $Log 'DISCONNECTED'
}

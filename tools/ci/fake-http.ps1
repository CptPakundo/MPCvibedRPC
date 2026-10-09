# One local web server playing two parts: MPC-HC's /variables.html (state read from $StateFile: 0 stopped, 1 paused,
# 2 playing) and GitHub's "latest release" API plus the download of the new program.
param([Parameter(Mandatory)][int]$Port, [Parameter(Mandatory)][string]$StateFile, [Parameter(Mandatory)][string]$NewExe, [string]$Tag = '0.9.7')
$ErrorActionPreference = 'Stop'
$l = New-Object System.Net.HttpListener
$l.Prefixes.Add("http://127.0.0.1:$Port/")
$l.Start()
$clock = [Diagnostics.Stopwatch]::StartNew()
while ($l.IsListening) {
  $c = $l.GetContext()
  $p = $c.Request.Url.AbsolutePath
  $res = $c.Response
  try {
    if ($p -eq '/variables.html') {
      $state = (Get-Content $StateFile -Raw).Trim()
      $html = '<html><body><p id="file">Sample.Movie.2001.1080p.mkv</p><p id="filepath">C:\Movies\Sample.Movie.2001.1080p.mkv</p><p id="state">' + $state + '</p><p id="position">' + (1000 + [int]$clock.ElapsedMilliseconds) + '</p><p id="duration">7500000</p><p id="playbackrate">1</p></body></html>'
      $bytes = [Text.Encoding]::UTF8.GetBytes($html); $res.ContentType = 'text/html'
    } elseif ($p -like '*/releases/latest') {
      $json = '{"tag_name":"v' + $Tag + '","body":"test release","html_url":"http://x","assets":[{"name":"MPCvibedRPC.exe","browser_download_url":"http://127.0.0.1:' + $Port + '/dl"}]}'
      $bytes = [Text.Encoding]::UTF8.GetBytes($json); $res.ContentType = 'application/json'
    } elseif ($p -eq '/dl') {
      $bytes = [IO.File]::ReadAllBytes($NewExe); $res.ContentType = 'application/octet-stream'
    } else {
      $res.StatusCode = 404; $bytes = [Text.Encoding]::UTF8.GetBytes('not found')
    }
    $res.ContentLength64 = $bytes.Length
    $res.OutputStream.Write($bytes, 0, $bytes.Length)
  } catch {} finally { try { $res.OutputStream.Close() } catch {} }
}

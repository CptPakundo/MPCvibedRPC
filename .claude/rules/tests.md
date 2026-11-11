---
paths:
  - "**/*_test.go"
  - "tools/ci/**"
---
# Writing tests
- `tools/ci/test-noskip.sh` fails CI when a test skips that should run on that OS. A new test that is Linux-only,
  Windows-only or opt-in must be added to `tools/ci/allowed-skips.sh` (ci.yml and release.yml both use it). The live
  test scripts run through `must-pass.sh`, so a live test that skips fails its CI job.
- Engine tests use a fake Discord (Unix socket on Linux, named pipe on Windows) and fake players (`players_test.go`,
  `vlc_test.go`). Timing matters: wait for state with the `eventually()` helper rather than reading it once. They run
  in parallel and poll every 25 ms (`newTestEngine` sets `Options.MinPoll`; `ReclearEvery`, `RecheckAfter` and
  `RetryAfter` shorten the other waits), so a test must never change a package variable; the one that has to (it shortens
  `discord.RequestTimeout`) stays sequential. Pure timer logic is tested on `testing/synctest`'s clock
  (`TestNudgesArePaced`).
- `Options.Pipes` keeps a real local MPC-QT/mpv/IINA out of the tests and `Options.MPRIS` the real session bus; VLC is
  only asked when a test sets a password. On macOS a socket path may only be 104 bytes, so test sockets go in a short
  folder under /tmp there (`sockDir`).
- Run `go test -race` somewhere that supports it (Linux CI) after concurrency changes.

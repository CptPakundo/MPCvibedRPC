---
paths:
  - "internal/capture/**"
  - "internal/engine/**"
---
**The stream title** (`streamTitle`: "auto", "always", "off"): Discord names a stream after a Playing activity only.
`internal/capture` tells whether Discord shares the screen from the thread its app runs only then (`shareThreads`:
`RsVideoSource`, seen on a real stream with Discord 1.0.9267) and, as a second sign, from Windows' record of screen
captures, which Windows 11 25H2 and later no longer write where it can be read (found 2026-11-11). The engine
builds the Playing form only then. A Discord that renames the thread makes "auto" see nothing: the fix is the name in
`shareThreads`, found by listing Discord's thread names while a share starts and stops. Engine tests never look at
this computer (`Options.Sharing`; the engine tests' `init` replaces the default); the capture package's own tests
read this computer's Discord, read-only. It cannot be tried against a real Discord in tests: the owner checks it on a real
stream.

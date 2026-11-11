---
paths:
  - "internal/capture/**"
  - "internal/engine/**"
---
**The stream title** (`streamTitle`: "auto", "always", "off"): Discord names a stream after a Playing activity only.
`internal/capture` tells from Windows' record of screen captures whether Discord shares the screen; the engine
builds the Playing form only then. Tests never read this computer's record (`Options.Sharing`; the engine tests'
`init` replaces the default). It cannot be tried against a real Discord in tests: the owner checks it on a real
stream.

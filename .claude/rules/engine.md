---
paths:
  - "internal/engine/**"
---
# The engine
- Resting (`engine.rest`: Discord's endpoint absent and no window open) asks no player and stops the watchers; tests get
  no resting unless they set `MayRest`. Clearing (`tick.go`): a sent status counts as shown until a clear is confirmed,
  and an idle status is cleared again every minute; `internal/engine/stale_test.go` pins it. The engine's state rules
  (what makes a status due, what a stale run may write, the retry wait) are in `docs/development.md` > The engine's
  state; after a change to the tick run `MPCRPC_CONVERGE_SEEDS=500 go test -run TestStatusConverges ./internal/engine`.

---
name: new-catalog
description: Checklist for adding a new catalog service (artwork or episode titles) to MPCvibedRPC - core.Sources, the Base field, providers, blocked hosts, the canary subtest, SECURITY.md and credits.
---
**New catalog service:** an entry in `core.Sources` (id, label, use, host: it gets its switch on the Privacy tab), a
`<Name>Base` field with its default in `core.Config` (tests point it at a fake), its id in the default
`artworkSources` and/or `episodeSources` (a default change needs the owner), the `Provider` in
`artwork/providers.go` and/or the `epProvider` in `artwork/episodes.go`, its line in `blockedHostFilter`
(`artwork/disabled.go`), a subtest in `internal/artwork/live_test.go` (the canary), its row in `SECURITY.md`, the
credit its terms ask for (Help tab credits, `THIRD-PARTY-NOTICES.md`) and `docs/config.md`.

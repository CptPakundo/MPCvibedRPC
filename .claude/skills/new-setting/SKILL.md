---
name: new-setting
description: Checklist for adding a new setting to MPCvibedRPC - core.Config and its default, the store schema, docs/config.md, diag for private values.
---
**New setting:** a field with a `json` tag in `core.Config` and its default in `DefaultConfig()`; a `Field` in
`store/schema.go` if it belongs in the window (int fields need Min/Max; `secret` for passwords and keys); a row in
`docs/config.md`; `diag.summarise` if the value is private (shown as "(set)"). Defaults that change behaviour for
existing users need the owner's decision and a line in the release notes.

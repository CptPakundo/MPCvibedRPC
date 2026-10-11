# Contributing

The program is AI-generated ("vibe coded"): the code, tests and docs were written with Claude from the owner's prompts
and testing. Bug reports, reviews and pull requests with tests are welcome all the same.

- **Bugs:** use the [issue templates](https://github.com/CptPakundo/MPCvibedRPC/issues/new/choose). On the **Help** tab,
  **Copy diagnostics** gives a report to attach. It names what you played (folder paths, sign-ins and keys are left out); look through it before you post it.
- **Security problems:** report them privately, as described in [SECURITY.md](SECURITY.md), not in a public issue.
- **Building and testing:** see [docs/development.md](docs/development.md). `go vet ./...` and `go test ./...` are the
  basics; CI runs the rest.
- **Pull requests:** keep them focused, and update the README and `docs/` when behaviour changes.
- **Privacy:** no personal data in code, tests, docs or screenshots: no email addresses, local paths or user names,
  and no real show or movie names. Use made-up titles such as "Sample Show S01E02".

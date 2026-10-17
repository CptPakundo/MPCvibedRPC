# Contributing

The program is AI-generated ("vibe coded"): the code, tests and docs were written with Claude from the owner's prompts
and testing. Bug reports, reviews and pull requests with tests are welcome all the same.

- **Bugs:** use the [issue templates](https://github.com/CptPakundo/MPCvibedRPC/issues/new/choose). On the **Help** tab,
  **Copy diagnostics** gives a report to attach. It names what you played (folder paths, sign-ins and keys are left out); look through it before you post it.
- **Security problems:** report them privately, as described in [SECURITY.md](SECURITY.md), not in a public issue.
- **Building and testing:** see [docs/development.md](docs/development.md). `go vet ./...` and `go test ./...` are the
  basics; CI runs the rest.
- **Pull requests:** keep them focused, and update the README and `docs/` when behaviour changes. Check
  [docs/decisions.md](docs/decisions.md) and the roadmap's [Not goals](docs/roadmap.md#not-goals) first: a change that
  goes against one should say why.
- **AI-assisted pull requests** are welcome, as the program itself is made that way ([how](docs/ai.md)). Say which
  parts an AI wrote, and what you tested for real (not only against fakes); add tests for the change. A pull request
  you have not run yourself will most likely be closed.
- **Privacy:** no personal data in code, tests, docs or screenshots: no email addresses, local paths or user names,
  and no real show or movie names. Use made-up titles such as "Sample Show S01E02".

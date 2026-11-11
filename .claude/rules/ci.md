---
paths:
  - ".github/**"
  - "tools/ci/**"
---
# CI files
- **Actions are pinned by commit** (`uses: owner/action@<sha> # vX.Y.Z`); Dependabot updates the pins. A new action
  gets the same form, and a job gets only the `permissions` it needs (workflows are read-only by default).
- The CI scripts share `tools/ci/lib.sh` (the test video, `wait_socket`, `wait_bus`, `done_file`);
  `run-annotated.sh` fails a step whose script did not reach `done_file` when `PASSED_FILE` is set.
- How the jobs fit together: `docs/development.md` > CI.

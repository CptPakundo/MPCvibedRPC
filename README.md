# Download counter data

Written by `.github/workflows/downloads.yml` and `tools/ci/dev-download.sh` on `master`; nothing here is code.

- `baseline.json`: each release file's count when counting started.
- `ledger/`: downloads made by development (release checks, Claude agents), one file per recording.
- `downloads.json`: the README badge (shields.io endpoint format).
- `summary.json`: people's downloads since the baseline, by file and release, and the development downloads left out.

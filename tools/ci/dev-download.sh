#!/usr/bin/env bash
# Every download of a release file made by development (the release checks, Claude agents) goes through here, so the
# download counter (tools/downloads, .github/workflows/downloads.yml) can leave it out. Each recording is a file of
# its own in ledger/ on the stats branch, written through the GitHub API, so writers never conflict.
#   dev-download.sh download BY TAG DIR [PATTERN]   gh release download into DIR (new or empty), then records it
#   dev-download.sh record BY TAG/FILE[=N] ...      records downloads made another way (the program's own updater)
# BY says who downloaded ("release check v0.9.39 draft", "Claude agent"). Needs gh signed in with write access.
set -euo pipefail
repo="${GITHUB_REPOSITORY:-CptPakundo/MPCvibedRPC}"

record() {
  local by="$1"; shift
  local files="" item key n
  for item in "$@"; do
    key="${item%=*}"; n=1
    [ "$key" = "$item" ] || n="${item##*=}"
    files="$files${files:+,}\"$key\":$n"
  done
  local at name body
  at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  name="ledger/$(date -u +%Y%m%dT%H%M%SZ)-$RANDOM$RANDOM.json"
  body="{\"by\":\"$by\",\"at\":\"$at\",\"files\":{$files}}"
  for try in 1 2 3; do
    if gh api --method PUT "repos/$repo/contents/$name" -f branch=stats -f message="Ledger: $by" \
      -f content="$(printf '%s\n' "$body" | base64 | tr -d '\n')" > /dev/null; then
      echo "recorded: $body"
      return 0
    fi
    sleep $((try * 5))
  done
  echo "::warning title=Download ledger::could not record $body"
  return 1
}

case "${1:-}" in
  download)
    by="$2"; tag="$3"; dir="$4"; pattern="${5:-}"
    mkdir -p "$dir"
    if [ -n "$pattern" ]; then
      gh release download "$tag" --repo "$repo" --dir "$dir" --pattern "$pattern"
    else
      gh release download "$tag" --repo "$repo" --dir "$dir"
    fi
    items=()
    for f in "$dir"/*; do items+=("$tag/$(basename "$f")"); done
    record "$by" "${items[@]}"
    ;;
  record)
    shift
    record "$@"
    ;;
  *)
    echo "usage: dev-download.sh download BY TAG DIR [PATTERN] | record BY TAG/FILE[=N] ..." >&2
    exit 2
    ;;
esac

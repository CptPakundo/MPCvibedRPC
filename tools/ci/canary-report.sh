#!/usr/bin/env bash
# Turns the weekly canary's results into one issue (label "canary"): a failed run opens it, or comments on the one that
# is still open; a run that passes again comments and closes it. Each issue names the jobs that failed and links the run.
# NEEDS is the workflow's toJSON(needs); RUN_URL links the run; REPORT=1 acts, anything else only prints what it would do
# (a pull request that changes the canary, a run started by hand).
set -euo pipefail
failed="$(echo "$NEEDS" | jq -r 'to_entries[] | select(.value.result != "success" and .value.result != "skipped") | "- \(.key): \(.value.result)"')"
# A failed listing must not pass for "no issue is open": it would open a second issue, or never close the old one.
open="$(gh issue list --label canary --state open --json number --jq '.[0].number // empty')" || { echo "::error title=canary report::could not list the canary issues"; exit 1; }
run() { if [ "${REPORT:-}" = 1 ]; then "$@"; else printf 'would run:'; printf ' %q' "$@"; echo; fi; }

if [ -n "$failed" ]; then
  body="$(printf 'The weekly canary failed: a player, server or catalog service may have changed.\n\n%s\n\nRun: %s\n' "$failed" "$RUN_URL")"
  echo "$body"
  if [ -n "$open" ]; then
    run gh issue comment "$open" --body "$body"
  else
    run gh label create canary --color fbca04 --description "The weekly canary found an outside change" --force
    run gh issue create --title "Weekly canary failed" --label canary --body "$body"
  fi
else
  echo "Every canary job passed."
  if [ -n "$open" ]; then
    run gh issue comment "$open" --body "The weekly canary passes again: $RUN_URL"
    run gh issue close "$open"
  fi
fi
echo ok > "${PASSED_FILE:-/dev/null}"

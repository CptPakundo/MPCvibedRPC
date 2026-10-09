#!/usr/bin/env bash
# Runs go test verbosely, prints only what matters (failures, skips, package results), and fails when a test skipped
# that is not expected to skip on this system: a fake Discord or player that could not start would otherwise pass
# silently.   usage: test-noskip.sh 'TestA|TestB' go test args...
allowed="$1"; shift
out="$(mktemp)"
"$@" -v > "$out" 2>&1
code=$?
grep -v -E '^\s*(=== (RUN|PAUSE|CONT|NAME)|--- PASS)' "$out"
skipped="$(grep -E '^\s*--- SKIP: ' "$out" | sed -E 's/^\s*--- SKIP: ([^ ]+).*/\1/' | grep -v -x -E "${allowed}" || true)"
if [ -n "$skipped" ]; then
  echo "Unexpected skips (these tests must run here):"
  echo "$skipped"
  [ "$code" -eq 0 ] && code=1
fi
exit "$code"
#!/usr/bin/env bash
# Runs a verbose go test command and fails unless the test NAME ran and passed: a live test that skips (its fake
# Discord could not start) or matches nothing would otherwise count as a pass.   usage: must-pass.sh NAME go test -v ...
name="$1"; shift
out="$(mktemp)"
"$@" 2>&1 | tee "$out"
code=${PIPESTATUS[0]}
[ "$code" -eq 0 ] || exit "$code"
if ! grep -q -- "--- PASS: $name " "$out"; then
  echo "$name did not run and pass (skipped, or no test matched)"
  exit 1
fi

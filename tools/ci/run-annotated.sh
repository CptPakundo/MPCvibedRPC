#!/usr/bin/env bash
# Runs a command; when it fails, puts the tail of its output into a workflow annotation so the failure can be read
# through the check-runs API without downloading job logs.   usage: run-annotated.sh "title" command args...
title="$1"; shift
out="$(mktemp)"
"$@" 2>&1 | tee "$out"
code=${PIPESTATUS[0]}
if [ "$code" -ne 0 ]; then
  msg="$(tail -n 120 "$out" | tr -d '\r' | cut -c1-400 | head -c 30000 | sed -e 's/%/%25/g' | awk 'BEGIN{ORS="%0A"} {print}')"
  echo "::error title=${title} (exit ${code})::${msg}"
fi
exit "$code"

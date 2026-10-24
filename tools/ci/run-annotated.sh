#!/usr/bin/env bash
# Runs a command; when it fails, puts the tail of its output into a workflow annotation so the failure can be read
# through the check-runs API without downloading job logs. When PASSED_FILE is set, the command must also have created
# that file: the CI scripts write it as their very last line, so one that died early (bash 3.2 on macOS once did that
# quietly) cannot count as a pass.   usage: [PASSED_FILE=...] run-annotated.sh "title" command args...
title="$1"; shift
out="$(mktemp)"
"$@" 2>&1 | tee "$out"
code=${PIPESTATUS[0]}
if [ "$code" -eq 0 ] && [ -n "${PASSED_FILE:-}" ] && [ ! -f "$PASSED_FILE" ]; then
  echo "::error title=${title}::the script stopped before the end"
  code=1
elif [ "$code" -ne 0 ]; then
  msg="$(tail -n 120 "$out" | tr -d '\r' | cut -c1-400 | head -c 30000 | sed -e 's/%/%25/g' | awk 'BEGIN{ORS="%0A"} {print}')"
  echo "::error title=${title} (exit ${code})::${msg}"
fi
exit "$code"

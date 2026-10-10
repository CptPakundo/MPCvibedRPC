#!/usr/bin/env bash
# Writes the program's version (from cmd/mpcvibedrpc/main.go) and the next patch version to $GITHUB_ENV as VERSION and
# NEXT, so the workflows never carry a version of their own. With a tag (v1.2.3) it also checks that the tag matches.
set -euo pipefail
v=$(sed -n 's/^var version = "\([0-9.]*\)".*/\1/p' cmd/mpcvibedrpc/main.go | tr -d '\r')
[[ "$v" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]] || { echo "::error title=version::no version like 1.2.3 in cmd/mpcvibedrpc/main.go"; exit 1; }
next="${BASH_REMATCH[1]}.${BASH_REMATCH[2]}.$((BASH_REMATCH[3] + 1))"
if [ -n "${1:-}" ] && [ "${1#v}" != "$v" ]; then
  echo "::error title=version::the tag $1 does not match the version $v in cmd/mpcvibedrpc/main.go"; exit 1
fi
echo "VERSION=$v" >> "$GITHUB_ENV"
echo "NEXT=$next" >> "$GITHUB_ENV"
echo "version $v, next $next"

#!/usr/bin/env bash
# Rebuilds the release files the way the release builds them, so a release can be checked byte for byte.
#   reproduce.sh <version> <out-dir>        builds MPCvibedRPC.exe, MPCvibedRPC-linux-amd64 and -arm64 into out-dir
#   reproduce.sh compare <file> <file>      fails unless both files are the same
#   reproduce.sh hash <file> <sha256>       fails unless the file has that SHA-256
# The build needs the repository's files as they are committed (LF line endings) and the same Go version; no cgo.
set -euo pipefail

sha() { sha256sum "$1" | cut -d' ' -f1; }

case "${1:-}" in
  compare)
    a=$(sha "$2"); b=$(sha "$3")
    echo "$a  $2"
    echo "$b  $3"
    if [ "$a" != "$b" ]; then
      echo "::error title=not reproducible::$2 and $3 differ ($(cmp "$2" "$3" | head -1 || true))"
      exit 1
    fi
    echo "same"
    ;;
  hash)
    got=$(sha "$2")
    echo "$got  $2 (release: $3)"
    if [ "$got" != "$3" ]; then
      echo "::error title=not reproducible::the rebuilt $(basename "$2") differs from the release's"
      exit 1
    fi
    ;;
  *)
    v="$1"; out="$2"
    mkdir -p "$out"
    export CGO_ENABLED=0
    syso=cmd/mpcvibedrpc/rsrc_windows_amd64.syso
    go run ./tools/mkrsrc -version "$v" -out "$syso"
    GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-H=windowsgui -s -w -X main.version=$v" -o "$out/MPCvibedRPC.exe" ./cmd/mpcvibedrpc
    rm -f "$syso"
    for arch in amd64 arm64; do
      GOOS=linux GOARCH=$arch go build -trimpath -ldflags "-s -w -X main.version=$v" -o "$out/MPCvibedRPC-linux-$arch" ./cmd/mpcvibedrpc
    done
    ls -l "$out"
    ;;
esac

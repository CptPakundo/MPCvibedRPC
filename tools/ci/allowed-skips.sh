#!/usr/bin/env bash
# Prints the tests that may skip on this runner's system, for test-noskip.sh (ci.yml and release.yml share it).
# The live tests are opt-in everywhere; on Windows the Unix-socket fakes and the end-to-end run are Linux/macOS-only
# (the smoke test covers them there). A new test that skips on some system belongs here.
live='TestLivePlayer|TestLiveJellyfin|TestLivePotPlayer|TestLiveCatalogs'
case "${RUNNER_OS:-$(uname -s)}" in
  Windows|MINGW*|MSYS*) echo "$live|TestEndToEnd|TestLoginAndActivity|TestErrorReply|TestHandshakeTimeout|TestServerCloseFailsPending|TestCloseFrameSaysWhy|TestAutostartRegistryRoundTrip" ;;
  *) echo "$live" ;;
esac

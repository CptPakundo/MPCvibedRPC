#!/usr/bin/env bash
# Builds MPRP.app, one program for Apple silicon and Intel Macs, and zips it as OUTDIR/MPRP-macos.zip.
# Runs on macOS (it needs lipo, codesign and ditto).   usage: package-macos.sh VERSION OUTDIR
set -euo pipefail
version="$1"; out="$2"
work="$(mktemp -d)"
app="$work/MPRP.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources" "$out"
out="$(cd "$out" && pwd)" # the zip is written from inside $work

# cgo: the menu bar icon and the settings window are Cocoa code (internal/winsys/macapp_darwin.go)
for arch in arm64 amd64; do
  carch="$arch"; [ "$arch" = amd64 ] && carch=x86_64
  CGO_ENABLED=1 GOOS=darwin GOARCH="$arch" CC="clang -arch $carch" MACOSX_DEPLOYMENT_TARGET=13.0 \
    go build -trimpath -buildvcs=false -ldflags "-s -w -X main.version=$version" -o "$work/MPRP-$arch" ./cmd/mprp
  # the Cocoa part must be in both: a build without it would quietly fall back to the browser and no menu bar icon
  otool -L "$work/MPRP-$arch" | grep -q WebKit.framework || { echo "the $arch build lacks the macOS app code"; exit 1; }
done
lipo -create -output "$app/Contents/MacOS/MPRP" "$work/MPRP-arm64" "$work/MPRP-amd64"
go run ./tools/mkico -icns "$app/Contents/Resources/MPRP.icns"

# LSUIElement: no Dock icon or menu bar of its own (the program has no windows besides the settings page).
cat > "$app/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleDevelopmentRegion</key>
	<string>en</string>
	<key>CFBundleDisplayName</key>
	<string>MPRP</string>
	<key>CFBundleExecutable</key>
	<string>MPRP</string>
	<key>CFBundleIconFile</key>
	<string>MPRP</string>
	<key>CFBundleIdentifier</key>
	<string>io.github.cptpakundo.mprp</string>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>CFBundleName</key>
	<string>MPRP</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>$version</string>
	<key>CFBundleVersion</key>
	<string>$version</string>
	<key>LSMinimumSystemVersion</key>
	<string>13.0</string>
	<key>LSUIElement</key>
	<true/>
	<key>NSLocalNetworkUsageDescription</key>
	<string>MPRP reads what plays on the Plex, Jellyfin, Emby or Kodi server you set up on your network, to show it on Discord.</string>
	<key>NSHumanReadableCopyright</key>
	<string>MIT License. Made with AI (vibe coded).</string>
</dict>
</plist>
EOF
plutil -lint "$app/Contents/Info.plist"
# NSLocalNetworkUsageDescription: macOS 15 and later ask before an app reaches the local network (a Plex, Jellyfin,
# Emby or Kodi server there); the floor matches what the binaries were built for (Go 1.27 needs macOS 13)
plutil -extract NSLocalNetworkUsageDescription raw "$app/Contents/Info.plist" > /dev/null
[ "$(plutil -extract LSMinimumSystemVersion raw "$app/Contents/Info.plist")" = 13.0 ] || { echo "LSMinimumSystemVersion is not 13.0"; exit 1; }
for arch in arm64 amd64; do
  minos="$(otool -l "$work/MPRP-$arch" | awk '/LC_BUILD_VERSION/ { b = 1 } b && $1 == "minos" { print $2; exit }')"
  echo "$arch build: minimum macOS $minos"
  [ -n "$minos" ] && [ "${minos%%.*}" -le 13 ] || { echo "the $arch build needs macOS $minos, above the app's 13.0"; exit 1; }
done

# An ad-hoc signature (no identity): Apple silicon runs only signed code. It is not a Developer ID signature, so
# macOS asks the user to confirm the first start (see the README).
codesign --force --sign - --timestamp=none "$app"
codesign --verify --strict --verbose=2 "$app"
lipo -archs "$app/Contents/MacOS/MPRP"

rm -f "$out/MPRP-macos.zip"
(cd "$work" && ditto -c -k --keepParent MPRP.app "$out/MPRP-macos.zip")
ls -l "$out/MPRP-macos.zip"

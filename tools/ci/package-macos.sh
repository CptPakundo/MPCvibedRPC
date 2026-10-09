#!/usr/bin/env bash
# Builds MPCvibedRPC.app, one program for Apple silicon and Intel Macs, and zips it as OUTDIR/MPCvibedRPC-macos.zip.
# Runs on macOS (it needs lipo, codesign and ditto).   usage: package-macos.sh VERSION OUTDIR
set -euo pipefail
version="$1"; out="$2"
work="$(mktemp -d)"
app="$work/MPCvibedRPC.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources" "$out"

for arch in arm64 amd64; do
  CGO_ENABLED=0 GOOS=darwin GOARCH="$arch" go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$work/MPCvibedRPC-$arch" ./cmd/mpcvibedrpc
done
lipo -create -output "$app/Contents/MacOS/MPCvibedRPC" "$work/MPCvibedRPC-arm64" "$work/MPCvibedRPC-amd64"
go run ./tools/mkico -icns "$app/Contents/Resources/MPCvibedRPC.icns"

# LSUIElement: no Dock icon or menu bar of its own (the program has no windows besides the settings page).
cat > "$app/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleDevelopmentRegion</key>
	<string>en</string>
	<key>CFBundleDisplayName</key>
	<string>MPCvibedRPC</string>
	<key>CFBundleExecutable</key>
	<string>MPCvibedRPC</string>
	<key>CFBundleIconFile</key>
	<string>MPCvibedRPC</string>
	<key>CFBundleIdentifier</key>
	<string>io.github.cptpakundo.mpcvibedrpc</string>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>CFBundleName</key>
	<string>MPCvibedRPC</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>$version</string>
	<key>CFBundleVersion</key>
	<string>$version</string>
	<key>LSMinimumSystemVersion</key>
	<string>11.0</string>
	<key>LSUIElement</key>
	<true/>
	<key>NSHumanReadableCopyright</key>
	<string>MIT License. Entirely AI-generated (vibe coded).</string>
</dict>
</plist>
EOF
plutil -lint "$app/Contents/Info.plist"

# An ad-hoc signature (no identity): Apple silicon runs only signed code. It is not a Developer ID signature, so
# macOS asks the user to confirm the first start (see the README).
codesign --force --sign - --timestamp=none "$app"
codesign --verify --strict --verbose=2 "$app"
lipo -archs "$app/Contents/MacOS/MPCvibedRPC"

rm -f "$out/MPCvibedRPC-macos.zip"
(cd "$work" && ditto -c -k --keepParent MPCvibedRPC.app "$out/MPCvibedRPC-macos.zip")
ls -l "$out/MPCvibedRPC-macos.zip"

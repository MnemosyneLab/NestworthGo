#!/bin/zsh

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
OUTPUT_DIR="$ROOT_DIR/dist/macos"
APP_NAME="Nestworth"
APP_ID="com.nestworth.app"
RAW_APP_VERSION="${NESTWORTH_VERSION:-0.1.4}"
APP_VERSION="${RAW_APP_VERSION#v}"
APP_BUILD="${NESTWORTH_BUILD:-1}"
APP_PATH="$OUTPUT_DIR/$APP_NAME.app"
DMG_PATH="$OUTPUT_DIR/${APP_NAME}-${APP_VERSION}-arm64.dmg"
STAGING_DIR="$OUTPUT_DIR/dmg-root"
BINARY_PATH="$OUTPUT_DIR/$APP_NAME"

if [[ "$(uname -s)" != "Darwin" ]]; then
	print -u2 "macOS packaging requires Darwin; current host is $(uname -s)."
	exit 1
fi

if [[ "$(uname -m)" != "arm64" ]]; then
	print -u2 "This release script targets Apple Silicon arm64; current host is $(uname -m)."
	exit 1
fi

if [[ ! -x "$(command -v hdiutil)" ]]; then
	print -u2 "hdiutil is required to create the DMG."
	exit 1
fi

rm -rf "$OUTPUT_DIR/$APP_NAME.app" "$APP_PATH" "$DMG_PATH" "$STAGING_DIR" "$ROOT_DIR/$APP_NAME.app"
mkdir -p "$OUTPUT_DIR"

print "Building $APP_NAME $APP_VERSION (build $APP_BUILD) for darwin/arm64..."
GOCACHE="${GOCACHE:-/tmp/nestworth-go-build}" \
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 \
	go build -trimpath \
		-ldflags="-s -w -X github.com/waltwang/nestworth-go/internal/version.Version=v$APP_VERSION -X github.com/waltwang/nestworth-go/internal/version.Build=$APP_BUILD" \
		-o "$BINARY_PATH" ./cmd/nestworth

print "Packaging the .app with Fyne..."
GOCACHE="${GOCACHE:-/tmp/nestworth-go-build}" \
	go run fyne.io/fyne/v2/cmd/fyne package \
		--os darwin \
		--sourceDir "$ROOT_DIR/cmd/nestworth" \
		--executable "$BINARY_PATH" \
		--name "$APP_NAME" \
		--appID "$APP_ID" \
		--appVersion "$APP_VERSION" \
		--appBuild "$APP_BUILD" \
		--icon "$ROOT_DIR/assets/brand/app-icon.png" \
		--release

mv "$ROOT_DIR/$APP_NAME.app" "$APP_PATH"

# Keep the migrated native ICNS resource as the bundle icon. Fyne's PNG input
# remains the portable source used when regenerating icons for other targets.
cp "$ROOT_DIR/assets/icons/icon.icns" "$APP_PATH/Contents/Resources/icon.icns"

mkdir -p "$STAGING_DIR"
cp -R "$APP_PATH" "$STAGING_DIR/$APP_NAME.app"
ln -s /Applications "$STAGING_DIR/Applications"
cp "$ROOT_DIR/assets/icons/icon.icns" "$STAGING_DIR/.VolumeIcon.icns"
SetFile -a C "$STAGING_DIR"

print "Creating the DMG..."
hdiutil create \
	-volname "$APP_NAME" \
	-srcfolder "$STAGING_DIR" \
	-ov \
	-format UDZO \
	"$DMG_PATH" >/dev/null

PLIST="$APP_PATH/Contents/Info.plist"
test "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$PLIST")" = "$APP_ID"
test "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleName' "$PLIST")" = "$APP_NAME"
test "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$PLIST")" = "$APP_VERSION"
test "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleVersion' "$PLIST")" = "$APP_BUILD"
cmp -s "$ROOT_DIR/assets/icons/icon.icns" "$APP_PATH/Contents/Resources/icon.icns"
test "$(lipo -archs "$APP_PATH/Contents/MacOS/$APP_NAME")" = "arm64"
hdiutil imageinfo "$DMG_PATH" >/dev/null

rm -rf "$STAGING_DIR" "$BINARY_PATH"
print "Created: $APP_PATH"
print "Created: $DMG_PATH"
print "Signing status: unsigned (distribution signing/notarization is a separate release step)."

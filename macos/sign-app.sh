#!/bin/sh
# Signs Refract.app: every Mach-O inside it first, then the app that seals them.
#
# Usage: macos/sign-app.sh <Refract.app>
#   REFRACT_SIGN_IDENTITY  the signing identity, e.g. "Developer ID Application: Name (TEAMID)".
#                          Default "-": ad hoc, for development builds (not accepted by Gatekeeper).
#   REFRACT_KEYCHAIN       optional keychain to look the identity up in (CI uses a temporary one)
#
# Hardened runtime everywhere. No entitlements, on purpose: see macos/README.md, "Signing".
# A real identity also gets a secure timestamp, which notarization requires.
# Signatures live in the files, so symlinks (Git's hundreds of git-* links) are not signed.
# Each nested Mach-O gets a stable identifier, <bundle id>.<file name>: without one, codesign
# keeps the linker's, which carries a hash that changes with every build.
set -eu
[ $# -eq 1 ] && [ -d "$1/Contents" ] || { echo "usage: sign-app.sh <Refract.app>" >&2; exit 2; }
APP=$1
IDENTITY=${REFRACT_SIGN_IDENTITY:--}
if [ "$IDENTITY" = "-" ]; then
  TIMESTAMP=--timestamp=none
else
  TIMESTAMP=--timestamp
fi
BUNDLE_ID=$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$APP/Contents/Info.plist")

# sign <path> [codesign options...]
sign() {
  f=$1; shift
  if [ -n "${REFRACT_KEYCHAIN:-}" ]; then
    codesign --force --options runtime "$TIMESTAMP" --keychain "$REFRACT_KEYCHAIN" --sign "$IDENTITY" "$@" "$f"
  else
    codesign --force --options runtime "$TIMESTAMP" --sign "$IDENTITY" "$@" "$f"
  fi
}

if [ "$IDENTITY" = "-" ]; then
  echo "==> signing ad hoc (development build: Gatekeeper will not accept it)"
else
  echo "==> signing with: $IDENTITY"
fi
# Inside out. codesign --deep doesn't sign what is under Resources, so each Mach-O is
# signed here (the tools, then the server, then the app executable) and the app seals them.
find "$APP/Contents" -type f ! -path "$APP/Contents/MacOS/*" -print | while IFS= read -r f; do
  case "$(file -b "$f")" in
    *Mach-O*) sign "$f" --identifier "$BUNDLE_ID.$(basename "$f")" ;;
  esac
done
# The app's executable and the app take the bundle id from Info.plist.
find "$APP/Contents/MacOS" -type f -print | while IFS= read -r f; do sign "$f"; done
sign "$APP"

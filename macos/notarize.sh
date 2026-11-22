#!/bin/sh
# Notarizes a signed Refract.app with Apple, staples the ticket and checks that Gatekeeper
# accepts it. Run locally (a keychain profile) or in CI (an App Store Connect API key).
#
# Usage: macos/notarize.sh <Refract.app>
# Credentials, from the environment (values are never printed):
#   REFRACT_NOTARY_PROFILE   a notarytool keychain profile, created once with
#                            `xcrun notarytool store-credentials` (local use)
#   REFRACT_NOTARY_KEY_PATH  path to the App Store Connect API key (.p8), with
#   REFRACT_NOTARY_KEY_ID    its key id and
#   REFRACT_NOTARY_ISSUER    the issuer id (CI)
# Without either set the script stops before it uploads anything.
set -eu
[ $# -eq 1 ] && [ -d "$1/Contents" ] || { echo "usage: notarize.sh <Refract.app>" >&2; exit 2; }
APP=$(cd "$1" && pwd)
{ set +x; } 2>/dev/null   # never trace the credentials below, even under sh -x

if [ -n "${REFRACT_NOTARY_PROFILE:-}" ]; then
  set -- --keychain-profile "$REFRACT_NOTARY_PROFILE"
elif [ -n "${REFRACT_NOTARY_KEY_PATH:-}" ] && [ -n "${REFRACT_NOTARY_KEY_ID:-}" ] && [ -n "${REFRACT_NOTARY_ISSUER:-}" ]; then
  [ -f "$REFRACT_NOTARY_KEY_PATH" ] || { echo "error: REFRACT_NOTARY_KEY_PATH is not a file" >&2; exit 2; }
  set -- --key "$REFRACT_NOTARY_KEY_PATH" --key-id "$REFRACT_NOTARY_KEY_ID" --issuer "$REFRACT_NOTARY_ISSUER"
else
  echo "error: set REFRACT_NOTARY_PROFILE, or REFRACT_NOTARY_KEY_PATH with REFRACT_NOTARY_KEY_ID and REFRACT_NOTARY_ISSUER" >&2
  exit 2
fi

# Notarization only takes a Developer ID signature: say so before the upload, not after.
"$(dirname "$0")/verify-app.sh" --release "$APP"

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT
ZIP="$WORK/Refract.zip"
echo "==> zipping"
ditto -c -k --keepParent "$APP" "$ZIP"

echo "==> submitting to Apple (waits for the result, usually a few minutes)"
# An Invalid result also exits non-zero: read the JSON either way, to fetch the log.
xcrun notarytool submit "$ZIP" "$@" --wait --output-format json > "$WORK/result.json" || true
cat "$WORK/result.json"
# JSON without jq: plutil reads it.
STATUS=$(plutil -extract status raw -o - "$WORK/result.json" 2>/dev/null || echo unknown)
ID=$(plutil -extract id raw -o - "$WORK/result.json" 2>/dev/null || echo "")
if [ "$STATUS" != "Accepted" ]; then
  echo "error: notarization status: $STATUS" >&2
  [ -z "$ID" ] || xcrun notarytool log "$ID" "$@" || true
  exit 1
fi

echo "==> stapling"
xcrun stapler staple "$APP"
xcrun stapler validate "$APP"
echo "==> Gatekeeper"
spctl --assess --type execute -vv "$APP"
"$(dirname "$0")/verify-app.sh" --release "$APP"
echo "ok: $APP is signed, notarized and stapled"

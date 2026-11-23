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
#   REFRACT_NOTARY_TIMEOUT   how long to wait for Apple, e.g. 45m (default 2h)
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
# The exit status doesn't say whether Apple accepted it (an Invalid result can exit 0): the
# status in the JSON does. A stuck submission gives up after the timeout, its id printed.
xcrun notarytool submit "$ZIP" "$@" --wait --timeout "${REFRACT_NOTARY_TIMEOUT:-2h}" --output-format json > "$WORK/result.json" || true
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
# The ticket can take a moment to reach Apple's CDN after Accepted: retry a few times.
n=0
until xcrun stapler staple "$APP"; do
  n=$((n + 1))
  [ "$n" -lt 5 ] || { echo "error: stapling failed (submission $ID)" >&2; exit 1; }
  sleep 30
done
xcrun stapler validate "$APP"
echo "==> Gatekeeper"
spctl --assess --type execute -vv "$APP"
"$(dirname "$0")/verify-app.sh" --release "$APP"
echo "ok: $APP is signed, notarized and stapled"

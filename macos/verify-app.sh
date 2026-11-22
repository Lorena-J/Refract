#!/bin/sh
# Checks a signed Refract.app: the seal is valid, and every Mach-O inside it (codesign --deep
# doesn't look under Resources) is validly signed with the hardened runtime and no entitlements.
#
# Usage: macos/verify-app.sh [--release] <Refract.app>
#   --release  also fail on anything ad hoc, unsigned, without a secure timestamp, or not signed
#              by the same Developer ID Application team as the app itself
set -eu
RELEASE=0
if [ "${1:-}" = "--release" ]; then RELEASE=1; shift; fi
[ $# -eq 1 ] && [ -d "$1/Contents" ] || { echo "usage: verify-app.sh [--release] <Refract.app>" >&2; exit 2; }
APP=$1
fail() { echo "error: $*" >&2; exit 1; }

echo "==> verifying $APP"
codesign --verify --deep --strict --verbose=2 "$APP" || fail "the app's signature is not valid"

# A link out of the bundle, or to nothing, is rejected by notarization.
ROOT=$(cd "$APP" && pwd -P)
find "$APP" -type l -print | while IFS= read -r l; do
  t=$(realpath "$l" 2>/dev/null) || fail "dangling symlink: $l"
  case "$t" in "$ROOT"/*) ;; *) fail "symlink leaves the bundle: $l -> $t" ;; esac
done

TEAM=$(codesign -dvv "$APP" 2>&1 | sed -n 's/^TeamIdentifier=//p')
COUNT=0
LIST=$(mktemp)
trap 'rm -f "$LIST"' EXIT
find "$APP" -type f -print > "$LIST"
while IFS= read -r f; do
  case "$(file -b "$f")" in *Mach-O*) ;; *) continue ;; esac
  COUNT=$((COUNT + 1))
  codesign --verify --strict "$f" || fail "$f is not validly signed"
  info=$(codesign -dvv "$f" 2>&1) || fail "cannot read the signature of $f"
  printf '%s\n' "$info" | grep -q 'flags=.*runtime' || fail "$f lacks the hardened runtime"
  [ -z "$(codesign -d --entitlements - "$f" 2>/dev/null)" ] || fail "$f has entitlements (none are expected)"
  if [ "$RELEASE" = 1 ]; then
    printf '%s\n' "$info" | grep -q '^Signature=adhoc' && fail "$f is signed ad hoc"
    printf '%s\n' "$info" | grep -q '^Authority=Developer ID Application:' || fail "$f is not signed with a Developer ID Application certificate"
    printf '%s\n' "$info" | grep -q '^Timestamp=' || fail "$f has no secure timestamp"
    [ -n "$TEAM" ] && [ "$TEAM" != "not set" ] || fail "the app has no team identifier"
    [ "$(printf '%s\n' "$info" | sed -n 's/^TeamIdentifier=//p')" = "$TEAM" ] || fail "$f is signed by another team than the app"
  fi
done < "$LIST"
[ "$COUNT" -gt 0 ] || fail "no Mach-O found in $APP"
echo "ok: $COUNT Mach-O files, all validly signed with the hardened runtime$([ "$RELEASE" = 1 ] && echo " by team $TEAM")"

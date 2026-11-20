#!/bin/sh
# Installs the image scanners at pinned versions, checking each download
# against the checksum recorded here (copied from the release's checksums.txt).
# Syft lists what the image holds (SBOM); Grype matches that list against
# vulnerability databases. Both are Apache-2.0, by Anchore.
# Usage: scripts/install-scanners.sh <bin folder>     (Linux amd64 or arm64)
set -eu
SYFT_VERSION=1.56.0
GRYPE_VERSION=0.121.1
DEST=$1
case "$(uname -m)" in
  x86_64|amd64)
    arch=amd64
    syft_sha=1c2493cfcf52d3367d6cb25780d9190c111c2219c34c820574c43e8bae15c528
    grype_sha=bc3ea3f9e6fd56ac781c08c6c5e614271b79f438200757f6932085c19b0a4657 ;;
  aarch64|arm64)
    arch=arm64
    syft_sha=ace61b9397ae28189c674ae332d67a8074dafd71e87724dfb8b22412d8253bb5
    grype_sha=33670612aad351c48cf3c37f4a85d8b043d1b64b21e6034e675bb00b956d78c1 ;;
  *) echo "unsupported architecture $(uname -m)" >&2; exit 1 ;;
esac
mkdir -p "$DEST"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
fetch() { # name version sha
  file="$1_$2_linux_${arch}.tar.gz"
  curl -fsSL -o "$work/$file" "https://github.com/anchore/$1/releases/download/v$2/$file"
  echo "$3  $work/$file" | sha256sum -c - >/dev/null
  tar -xzf "$work/$file" -C "$DEST" "$1"
}
fetch syft "$SYFT_VERSION" "$syft_sha"
fetch grype "$GRYPE_VERSION" "$grype_sha"
"$DEST/syft" version | grep -q "^Version: *$SYFT_VERSION$"
"$DEST/grype" version | grep -q "^Version: *$GRYPE_VERSION$"
echo "syft $SYFT_VERSION and grype $GRYPE_VERSION in $DEST"

#!/bin/sh
# Installs the Tectonic and synctex that the Docker image ships, into <dir>,
# on Linux or macOS (amd64 or arm64). CI uses it so tests compile real documents with
# the same versions as production. The pins live in packaging/tools.lock, shared with
# the Dockerfile and the release job.
# Usage: scripts/install-tex-tools.sh <dir>   (needs curl, git, tar and a C compiler)
set -eu
. "$(dirname "$0")/tools-common.sh"
[ $# -eq 1 ] || die "usage: install-tex-tools.sh <dir>"
DIR=$1
case "$(uname -s)" in Linux) OS=linux ;; Darwin) OS=darwin ;; *) die "unsupported system $(uname -s)" ;; esac
case "$(uname -m)" in x86_64) ARCH=amd64 ;; aarch64 | arm64) ARCH=arm64 ;; *) die "unsupported architecture $(uname -m)" ;; esac
mkdir -p "$DIR"
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

asset=$(tectonic_asset "$OS" "$ARCH")
fetch "https://github.com/tectonic-typesetting/tectonic/releases/download/tectonic%40${TECTONIC_VERSION}/tectonic-${TECTONIC_VERSION}-${asset%|*}.${asset#*|}" \
  "$(lock_get TECTONIC_SHA256 "$OS" "$ARCH")" "$WORK/t.tgz"
tar -xzf "$WORK/t.tgz" -C "$DIR" tectonic

"$(dirname "$0")/build-synctex.sh" "$OS" "$ARCH" "$DIR/synctex"
rm -f "$DIR"/synctex.LICENSE.*

"$DIR/tectonic" --version
"$DIR/synctex" help 2>&1 | head -1

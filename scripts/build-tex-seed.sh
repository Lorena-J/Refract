#!/bin/sh
# Builds the TeX seed: the bundle files the built-in templates and the documents in
# packaging/tex-seed/documents need, as a cache fragment, plus its licences.
# Usage: scripts/build-tex-seed.sh <tectonic> <out-dir> [--update]
#   tectonic  the pinned Tectonic (its version must equal TECTONIC_VERSION in packaging/tools.lock)
#   out-dir   an empty folder: gets tex-seed/ and licenses/tex/ (see packaging/README.md)
#   --update  rewrite packaging/tex-seed/files.txt (the committed list of seed files)
# Needs network access (the bundle is fetched while compiling) and python3.
# The TeX Live database is fetched and checked against packaging/tools.lock; set
# TEX_SEED_TLPDB_XZ to a local copy of it to skip the download (still checked).
# shellcheck disable=SC1091,SC2154
set -eu
. "$(dirname "$0")/tools-common.sh"
[ $# -ge 2 ] || die "usage: build-tex-seed.sh <tectonic> <out-dir> [--update]"
TECTONIC=$1 OUT=$2
shift 2
command -v "$TECTONIC" >/dev/null 2>&1 || [ -x "$TECTONIC" ] || die "no tectonic at $TECTONIC"

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT
TLPDB=${TEX_SEED_TLPDB_XZ:-}
if [ -z "$TLPDB" ]; then
  TLPDB=$WORK/texlive.tlpdb.xz
  fetch "$TEXLIVE_TLPDB_URL" "$TEXLIVE_TLPDB_SHA256" "$TLPDB"
fi
TECTONIC=$(command -v "$TECTONIC" || echo "$TECTONIC")
case "$TECTONIC" in /*) ;; *) TECTONIC=$PWD/$TECTONIC ;; esac
python3 -I "$TOOLS_ROOT/scripts/tex-seed.py" build "$TECTONIC" "$OUT" "$TLPDB" "$@"

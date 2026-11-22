#!/bin/sh
# Builds a private, relocatable Git for macOS from the pinned official source
# tarball, on a macOS host of the target architecture (the release job uses
# macos-15 for arm64 and macos-15-intel for amd64).
# Usage: scripts/build-git-macos.sh <output-dir>
# Produces <output-dir>/bin/git, libexec/git-core/ (the helpers) and
# share/git-core/templates/, plus <output-dir>/COPYING and LGPL-2.1 and
# <output-dir>/source/git-<version>.tar.xz (the exact source, for the GPL offer).
#
# Choices: RUNTIME_PREFIX=YesPlease makes git find its helpers and templates
# relative to its own executable, so the folder can be moved. No curl, OpenSSL,
# expat, Perl, Python, Tcl/Tk or gettext: a local history needs none, and every
# library git still links (libSystem, libz, libiconv) ships with macOS, so the
# result needs no Homebrew and no bundled dylibs.
set -eu
. "$(dirname "$0")/tools-common.sh"
[ $# -eq 1 ] || die "usage: build-git-macos.sh <output-dir>"
[ "$(uname -s)" = Darwin ] || die "build-git-macos.sh runs on macOS"
OUT=$1
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

mkdir -p "$OUT/source"
fetch "https://mirrors.edge.kernel.org/pub/software/scm/git/git-${GIT_VERSION}.tar.xz" \
  "$GIT_SOURCE_SHA256" "$OUT/source/git-${GIT_VERSION}.tar.xz"
tar -xJf "$OUT/source/git-${GIT_VERSION}.tar.xz" -C "$WORK"
SRC="$WORK/git-${GIT_VERSION}"
STAGE="$WORK/stage"

make -C "$SRC" -j"$(sysctl -n hw.ncpu)" \
  prefix=/usr/local RUNTIME_PREFIX=YesPlease \
  NO_GETTEXT=YesPlease NO_OPENSSL=YesPlease NO_CURL=YesPlease NO_EXPAT=YesPlease \
  NO_TCLTCK=YesPlease NO_PERL=YesPlease NO_PYTHON=YesPlease NO_INSTALL_HARDLINKS=YesPlease \
  DESTDIR="$STAGE" install

mkdir -p "$OUT"
cp -R "$STAGE/usr/local/bin" "$STAGE/usr/local/libexec" "$STAGE/usr/local/share" "$OUT/"
# Tools a project's local history never runs.
rm -rf "$OUT/bin/git-shell" "$OUT/bin/scalar" "$OUT/bin/gitk" "$OUT/bin/git-cvsserver" \
  "$OUT/libexec/git-core/git-cvsserver" "$OUT/libexec/git-core/git-shell" \
  "$OUT/libexec/git-core/scalar" "$OUT/share/git-core/contrib" "$OUT/share/locale"
cp "$SRC/COPYING" "$SRC/LGPL-2.1" "$OUT/"
strip -x "$OUT/bin/git" "$OUT/libexec/git-core/git" 2>/dev/null || true

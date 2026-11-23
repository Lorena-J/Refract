#!/bin/sh
# Builds the synctex CLI from the pinned commit, with the pinned zlib compiled in
# statically, so the result depends on no zlib of the system.
# Usage: scripts/build-synctex.sh <os> <arch> <output-file>
#   os: darwin|linux|windows; arch: amd64|arm64 (the target).
# Compiler: $CC (default cc). Windows is cross-built: set CC to an llvm-mingw
# clang for the target (e.g. x86_64-w64-mingw32-clang). Needs curl, git, tar, ar.
set -eu
. "$(dirname "$0")/tools-common.sh"
[ $# -eq 3 ] || die "usage: build-synctex.sh <os> <arch> <output-file>"
OS=$1 ARCH=$2 OUT=$3
CC=${CC:-cc}
AR=${AR:-ar}
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

fetch "https://github.com/madler/zlib/releases/download/v${ZLIB_VERSION}/zlib-${ZLIB_VERSION}.tar.gz" \
  "$ZLIB_SHA256" "$WORK/zlib.tgz"
tar -xzf "$WORK/zlib.tgz" -C "$WORK"
ZSRC="$WORK/zlib-${ZLIB_VERSION}"

# macOS: the architecture of the target; the others are chosen by the compiler.
ARCHFLAG=
if [ "$OS" = darwin ]; then
  case "$ARCH" in amd64) ARCHFLAG="-arch x86_64" ;; arm64) ARCHFLAG="-arch arm64" ;; *) die "unsupported arch $ARCH" ;; esac
fi
UNIX_DEF=-DHAVE_UNISTD_H=1
[ "$OS" = windows ] && UNIX_DEF=

mkdir "$WORK/zobj"
for f in adler32 crc32 deflate infback inffast inflate inftrees trees zutil compress uncompr gzclose gzlib gzread gzwrite; do
  # shellcheck disable=SC2086
  $CC -O2 $ARCHFLAG $UNIX_DEF -I"$ZSRC" -c "$ZSRC/$f.c" -o "$WORK/zobj/$f.o"
done
"$AR" rcs "$WORK/libz.a" "$WORK"/zobj/*.o

git clone -q "$SYNCTEX_REPO" "$WORK/synctex"
git -C "$WORK/synctex" checkout -q --detach "$SYNCTEX_COMMIT"
cd "$WORK/synctex"
# Outside TeX Live, synctex_main.c expects <w2c/c-auto.h>; point it at the repo's
# own config header. (__SYNCTEX_WORK__ is the author's local setup and hardcodes
# a zlib path, so it is not used.)
mkdir -p w2c
echo '#include "synctex_parser_c-auto.h"' > w2c/c-auto.h
STATIC='' LIBS=''
[ "$OS" = linux ] && STATIC=-static
if [ "$OS" = windows ]; then
  # On Windows synctex_main.c calls kpathsea's kpse_set_program_name, which only
  # matters inside TeX Live; the parser itself uses shlwapi.
  mkdir -p kpathsea
  echo '#define kpse_set_program_name(argv0, name) ((void)0)' > kpathsea/progname.h
  STATIC=-static LIBS=-lshlwapi
fi
# shellcheck disable=SC2086
$CC -O2 $ARCHFLAG $STATIC -I. -I"$ZSRC" synctex_main.c synctex_parser.c synctex_parser_utils.c "$WORK/libz.a" $LIBS -o "$WORK/synctex.bin"
case "$OS" in
  darwin) strip -x "$WORK/synctex.bin" 2>/dev/null || true ;;
esac
mkdir -p "$(dirname "$OUT")"
cp "$WORK/synctex.bin" "$OUT"
# Licence texts travel with the binary: next to it, as <OUT>.LICENSE.synctex / .zlib.
cp LICENSE "$OUT.LICENSE.synctex"
cp "$ZSRC/LICENSE" "$OUT.LICENSE.zlib"

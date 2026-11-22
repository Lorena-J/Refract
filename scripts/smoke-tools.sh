#!/bin/sh
# Smoke test of an extracted tools archive, with a stripped environment: no PATH
# entries for the tools and none that could supply a system Git.
# Usage: scripts/smoke-tools.sh <extracted-dir> <os>   (os: darwin|linux|windows)
# Runs each tool's --version (synctex: help), checks the executables bring their
# libraries (macOS: only OS libraries; Linux: static), and for Git creates a
# project repository and a checkpoint using only the bundled runtime.
set -eu
DIR=$(cd "$1" && pwd)
OS=$2
fail() { echo "smoke: $*" >&2; exit 1; }

EXE=
case "$OS" in
  windows)
    EXE=.exe
    WINDIR_PATH=${SYSTEMROOT:-/c/Windows}/System32
    run() { env -i "SYSTEMROOT=${SYSTEMROOT:-C:\\Windows}" "PATH=$WINDIR_PATH" "$@"; }
    ;;
  *) run() { env -i PATH=/nonexistent "$@"; } ;;
esac

run "$DIR/tectonic$EXE" --version | head -1
run "$DIR/rg$EXE" --version | head -1
# synctex has no --version: `help` prints its banner (and exits non-zero on some builds).
run "$DIR/synctex$EXE" help 2>&1 | head -1 | grep -q SyncTeX || fail "synctex help"

case "$OS" in
  darwin)
    for f in tectonic synctex rg git/bin/git git/libexec/git-core/git; do
      bad=$(otool -L "$DIR/$f" | tail -n +2 | awk '{print $1}' | grep -v -e '^/usr/lib/' -e '^/System/Library/' || true)
      [ -z "$bad" ] || fail "$f links libraries that are not part of macOS: $bad"
    done
    ;;
  linux)
    for f in tectonic synctex rg; do
      ldd "$DIR/$f" 2>&1 | grep -q -e 'not a dynamic' -e 'statically linked' || fail "$f is not static"
    done
    ;;
esac

[ "$OS" = linux ] && { echo "smoke: ok ($OS)"; exit 0; }

case "$OS" in
  windows) GIT="$DIR/git/cmd/git.exe" ;;
  *) GIT="$DIR/git/bin/git" ;;
esac
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT
g() { run "HOME=$WORK" "USERPROFILE=$WORK" "$GIT" -C "$WORK/project" -c user.name=Refract -c user.email=refract@example.invalid "$@"; }
run "$GIT" --version
mkdir "$WORK/project"
g init -q
echo hello > "$WORK/project/main.tex"
g add main.tex
g commit -q -m "first checkpoint"
echo more >> "$WORK/project/main.tex"
g commit -q -a -m "second checkpoint"
test "$(g rev-list --count HEAD)" = 2 || fail "git history"
g status --porcelain | grep -q . && fail "git status is not clean"
g tag checkpoint-1
g gc -q
# The helpers come from the bundled folder, not from a system path.
case "$(g --exec-path)" in
  *"$(basename "$DIR")"* | "$DIR"/* | *libexec/git-core | *libexec\\git-core) ;;
  *) fail "git exec path is outside the bundle: $(g --exec-path)" ;;
esac
echo "smoke: ok ($OS)"

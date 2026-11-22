# Refract for macOS

A Swift app that runs `refract local --embedded` (the contract is in the top-level README,
"For packagers") and shows it in a `WKWebView`. The Go server stays the only backend.

```
macos/
  Package.swift            SwiftPM package: RefractCore (library), Refract (app), tests
  Sources/RefractCore/     the server's lifecycle, no UI: status line, supervisor, logs, bundle layout
  Sources/Refract/         AppKit: window, loading/error views, WKWebView, quit sequence, minimal menu
  Resources/               Info.plist and the English/Spanish strings
  Tests/RefractCoreTests/  lifecycle tests against a fake server script
  build-app.sh             assembles and signs Refract.app (run by `make mac`)
```

## Decisions

**SwiftPM and an assembly script, not an Xcode project.** A hand-written `.pbxproj` is
fragile and unreadable in review, `xcodebuild` adds nothing the app needs yet, and
SwiftPM builds and tests from the command line (`swift build`, `swift test`). The script
puts the bundle together. #69 (signing) and #70 (Sparkle) work from the same script. If a
feature ever needs an Xcode-only capability (asset catalogs for the icon, extensions), generate the
project from this package then.

**Minimum macOS 13 (Ventura).** The app needs `callAsyncJavaScript` (11), `WKDownload` (11.3) and
`WKHTTPCookieStore` (10.13), and the Go 1.27 binary is built for 13 (checked with `vtool`). 13 keeps the audience wide without
old-API workarounds, and it is the maintainer's to raise; the Swift app, the Go binary and, through
`MACOSX_DEPLOYMENT_TARGET` in `scripts/tools-common.sh`, Git and SyncTeX target it (`smoke-tools.sh`
checks the tools). Tectonic and ripgrep are upstream builds that target 11.

**One app per architecture.** The tools are built on a Mac of their own architecture (Git and
SyncTeX are compiled, not downloaded), so a universal app would need both archives on one machine
and `lipo` for every Mach-O, doubling the download (about 100 MB per architecture) for nothing the
person gains: an update channel (#70) and a Homebrew cask serve each architecture its own app.
`make mac MAC_ARCH=amd64` builds the Intel app on an Intel Mac, or with a `TOOLS_ARCHIVE` built
there; Go and Swift cross-compile.

**Layout: `Contents/Resources/Server/{bin/refract,libexec/refract/...}`, not `Contents/Helpers`.**
`internal/tools` (#53) finds the tools at `<exe dir>/../libexec/refract`, and this layout keeps that
rule unchanged. `Helpers/` was the candidate, and it fails: codesign's default rules treat every
file under `Helpers/` (like `MacOS/` and `Frameworks/`) as nested code, and Git's tree holds shell
scripts, templates and licences, so `codesign` refuses to seal the app (`code object is not signed
at all, in subcomponent: .../LICENSE-MIT`). Under `Resources/` the files are sealed as resources: the
build signs every Mach-O first (hardened runtime), then the app, and
`codesign --verify --deep --strict` passes. Changing one byte of a tool afterwards makes the
verification fail (checked). `--deep` does not look at signatures of code under `Resources/`, so
`build-app.sh` also verifies each Mach-O, and #69's check ("no unsigned Mach-O") must scan the whole
bundle the same way. Notarization itself is not tested here (#69).

**Same data folder as `refract local`.** The app sets no `REFRACT_DATA_DIR` (and drops the
variables `refract local` refuses), so the server uses `~/Library/Application Support/Refract`.

**A remembered port.** The web app keeps settings in the browser, per origin, and the origin includes
the port, so a different free port at every launch would forget them. The app asks for the port it
used last (`--port`) and, if it is taken, goes back to a free one.

## The server's life

- Start: token on a pipe (descriptor 3), standard input a pipe the app keeps, standard output read
  for one JSON line, standard error written straight to `~/Library/Logs/Refract/refract.log`
  (a file over 2 MB is rotated at the next start; two old files are kept). The child has its own
  process group and inherits nothing else from the app.
- `ready`: the token goes into the web view's cookie store (`refract_embed`, host only, HttpOnly,
  SameSite=Strict) and the page loads. `running` from a terminal: its URL is loaded as it is.
  `running` with `embedded`, `unresponsive`, `error` or no line: the error view.
- Quit (also `osascript -e 'quit app "Refract"'`): `window.refractFlush()`; if it answers
  false the person is asked; then standard input is closed, a progress view shows after one second,
  and the server is killed after 30 seconds. If the app itself dies, the pipe closes and the
  server stops by itself.
- The server exits after it was ready: the error view with the end of the log, Restart (stops what is
  left, starts again with a new token) and Show Logs. Projects and the database are not touched.
- A missing or non-executable server or tool, or `missing_tools` in `/api/me`: an error view with a
  link to download the app again (the bundle is signed and read-only: repairing is replacing it).

## Left for later

Links, downloads and file pickers (#67, `TODO(#67)` in `WebViewController`), the real menu (#68, the
current one is the minimum for Quit, Close and editing), signing and notarization (#69), updates (#70),
the icon.

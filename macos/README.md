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
  Either way, before the first load the web view gets a `WKContentRuleList` (`originOnlyRules` in
  RefractCore) that blocks every request outside that exact origin (scheme, host and port),
  subresources included, and allows only `blob:`, `data:` and `about:` besides: a host-only cookie
  goes to every port of the host, so this is what keeps the token away from other local services
  (project files are served under a `sandbox` CSP, which doesn't restrict loads). If the rules
  don't compile, nothing loads and the error view shows.
  `running` with `embedded`, `unresponsive`, `error` or no line: the error view.
- Quit (also `osascript -e 'quit app "Refract"'`): `window.refractFlush()`; if it answers
  false the person is asked; then standard input is closed, a progress view shows after one second,
  and the server's process group is killed after 30 seconds (a compile in flight runs in a group
  of its own, which the server stops when it shuts down normally but a kill doesn't reach). If the app itself dies, the pipe closes and the
  server stops by itself.
- The server exits after it was ready: the error view with the end of the log, Restart (stops what is
  left, starts again with a new token) and Show Logs. Projects and the database are not touched.
- A missing or non-executable server or tool, or `missing_tools` in `/api/me`: an error view with a
  link to download the app again (the bundle is signed and read-only: repairing is replacing it).

## Testing without touching real state

Environment variables of the app (not passed to the server) redirect what it writes outside the
data folder: `REFRACT_APP_LOG_DIR=<dir>` for the log folder and `REFRACT_APP_DEFAULTS_SUITE=<name>`
for a separate UserDefaults suite (the remembered port). The data folder is the server's: the app
never sets `REFRACT_DATA_DIR`, so to test the server with scratch data run the bundled binary directly,
`REFRACT_DATA_DIR=<dir> Refract.app/Contents/Resources/Server/bin/refract local --no-browser`.
Launch the app with them via `open --env` or `env ... Refract.app/Contents/MacOS/Refract`.

## The integrated window (#127)

The window has no title bar of its own (`.fullSizeContentView`, transparent title bar, hidden
title; the title stays for Mission Control, the Window menu and VoiceOver): the web app reaches the
top edge. **The app moves the traffic lights onto the bars' line, and the bars share one height.**
Every bar is 44 px tall and starts 8 px below the top edge (the page's padding), so its content row
is centred 30.5 pt down; `TrafficLights` (`RefractCore/WindowChrome.swift`) puts the buttons there,
18 pt from the left, which is inside the card on the Projects page and in the sidebar's grey on the
Workspace, so the lights never straddle an edge and header content and lights read as one row.
The alternative (a grey strip above each card) would add a dead band of 30 px to every screen.

- **Placing.** AppKit puts the buttons in a 28 pt title bar container; the app grows that container
  to 61 pt and moves the three buttons (`RootViewController.placeTrafficLights`). AppKit resets
  both on resize, full screen, appearance and screen changes, so the app places them again on those
  notifications and when the container's frame changes (the frame is written only when it differs).
- **Inset.** `WindowInset` measures where the zoom button ends. A `WKUserScript` at document start,
  and `evaluateJavaScript` when the window enters or leaves full screen, set `html[data-desktop="mac"]`
  and `--window-inset-left` / `--window-inset-top` (both 0 in full screen, where the buttons hide).
  Nothing sets them in a browser, so `index.css` rules under `[data-desktop]` never apply there. The
  first element of each `[data-window-bar]` clears the inset (buttons end + 12 pt). In the app the
  Workspace sidebar starts at 296 px (not 248) so the project name still fits next to the lights.
  The loading and error views sit in the safe area.
- **Dragging.** WKWebView has no `-webkit-app-region`. `web/src/lib/windowDrag.ts` posts
  `{type: "drag"}` to the `refractWindow` message handler on a mousedown on `[data-window-drag]`
  (not on buttons, links, inputs, tabs, `[data-no-drag]`); the app calls `performDrag(with:)`, and a
  double click follows `AppleActionOnDoubleClick`. Messages count only from the main frame of the
  app's origin (`WindowMessage.parse`). The handler exists only in the app.
- **Drag strip.** Screens without a top bar (the sign-in pages) and the overlay behind a dialog get
  a `.window-strip` element at the top (shown by one rule in `index.css`; delete the rule to go
  without it).

## Links, downloads, file pickers (#67)

What the web view does with things the page asks for (`WebViewController`; the pure decisions are in
`RefractCore/LinkPolicy.swift`, tested):

- **Links.** The server's own origin (same scheme, host and port, plus `about:blank`/`about:srcdoc`, `data:` and that origin's `blob:` URLs) stays in the
  app. `http`, `https` and `mailto` elsewhere open in the default browser through `NSWorkspace`, with the bare
  URL: the token cookie never leaves the web view. Any other scheme (`file:`, `javascript:`, custom schemes)
  is refused and logged, and so is another origin's `blob:`. Only the main frame opens the browser; a frame inside the page can't. `target="_blank"` and `window.open` never create a window: same-origin pages load in
  the main view (a `blob:`/`data:` one is ignored, it would replace the app), the rest follows the same rule. Sign-in pages (provider keys, GitHub's device flow, OIDC)
  therefore open in the browser.
- **Downloads** (Export .zip, Download PDF, a file's download). `WKDownload` for `shouldPerformDownload`
  (the web app's `a[download]` with a `blob:` URL works as it is, no web hook needed), for
  `Content-Disposition: attachment` and for responses the view can't show. A save panel opens as a sheet in
  Downloads with the suggested name (stripped of path parts, control and bidi characters and leading dots, cut to 255 bytes keeping the extension); a failure shows an alert; cancelling is silent.
- **File inputs.** `runOpenPanelWith`: multiple selection as asked; a directory input (`webkitdirectory`)
  gets a folder-only panel, the others a file-only one.
- **Drag and drop** onto the project list. WKWebView hands drops to the page. The project list now reads
  dropped folders through `webkitGetAsEntry` (`web/src/lib/dropfiles.ts`) and uploads them like the Folder
  picker; zips work as before.
- **`window.confirm`** (and `alert`/`prompt`, unused today) has a native sheet (without a delegate WebKit answers false, which would break the
  "leave this page" prompt).
- **Clipboard.** `navigator.clipboard.writeText` works from the loopback origin (a secure context). Reading
  (`readText`, the editor's Paste menu entry) shows WebKit's own "Paste" prompt.

Signing in to ChatGPT inside the app is not done here: today Settings → Assistant asks the person to run
`refract login` and upload the credentials file (the file picker works). The one-click flow from Settings
needs the server side of #61; its sign-in page then opens in the browser by the rule above.

## Left for later

The real menu (#68, the current one is the minimum for Quit, Close and editing), signing and notarization
(#69), updates (#70), the icon.

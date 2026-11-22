# Packaging policy

This folder covers the installable packages: the CLI packages that exist today
(Homebrew formula `refract`, WinGet manifests, `.deb`, portable archives) and the
desktop installers being added (macOS and Windows apps, Debian/Ubuntu package, Homebrew
cask `refract-app`). `server/` is a separate path for server deployments.

Keep the portable archives and their enclosing `refract-<tag>-<os>-<arch>/` directory:
WinGet's nested executable paths depend on that layout.

## What a package may contain

- The `refract` executable, which includes the server and its administration commands.
- Bundled tools, under a fixed layout: a `libexec/refract/` folder next to the binary's
  `bin/` folder (`../libexec/refract/` from the executable), and the same folder as the
  executable on Windows. #53 makes `refract` resolve its tools there. Desktop apps
  (macOS, Windows) ship private copies of Git with the helper executables and libraries
  it needs, Tectonic, SyncTeX and ripgrep (see "Bundled tools"). Debian/Ubuntu packages do not bundle Git:
  they declare it as a package dependency (`Depends: git`) and use the system copy.
- Licences and notices, and the documentation.
- A desktop entry and icons for graphical installs.
- Versions, checksums and redistribution terms (licences) of every bundled tool must be
  recorded with the release. #55 implements that record.
- A per-user service definition that is not enabled: a Homebrew `service` block for
  `brew services`, or a systemd user unit in `/usr/lib/systemd/user/`. It runs
  `refract local` as the person, and only once they start it.

Today's packages (Homebrew formula, WinGet, `.deb`, portable archives) contain only the
binary and documentation; #58–#60 add the bundled tools to them.

## What a package must not do

- Enable or start a service, or install a system-wide one. A per-user service runs only
  when the person starts it (`refract service install` (#57), `brew services start` or
  `systemctl --user enable`), never from an installer, maintainer script, Homebrew
  `post_install` or cask `postflight`.
- Create keys, databases, projects or any data folder at install time. `refract local`
  creates them on first run, in per-user locations.
- Store data, the key or caches inside the application directory: they live in
  per-user locations outside it, so replacing the app's executables leaves them intact.
- Run as root after installation, or ship setuid or world-writable files.
- Touch an existing data folder or the stored credentials on install, upgrade or
  removal. Uninstalling the app keeps the person's data.
- Add server configuration (secrets, listening addresses or sign-in settings).

The packaging tests (`scripts/test_packaging.py`) enforce the rules that can be
checked: no Debian maintainer scripts, no Homebrew `post_install` or cask `postflight`,
no system-wide service or data paths, no setuid or world-writable files, and a payload
that contains only the allowed paths.

## Bundled tools

`.github/workflows/tools.yml` builds a tools archive per target, `refract-tools-<tag>-<os>-<arch>`
(`.tar.gz`, `.zip` on Windows). The release attaches them and fails when an archive lacks a
required tool or licence (`scripts/tools-archive.py`, also run by the tests). The installers
(#58, #59, #60, #66) unpack one into the tools folder described above:

```
tectonic  synctex  rg            (.exe on Windows)
git/bin/git                      macOS only
git/libexec/git-core/            macOS only: the Git helpers (GIT_EXEC_PATH)
git/share/git-core/templates/    macOS only (GIT_TEMPLATE_DIR)
git/cmd/git.exe                  Windows: MinGit keeps its own tree (see below)
licenses/<component>/...         licences, notices, and for Git the source or its offer
tools-manifest.json              versions, sources and SHA-256 of the inputs and of every file
```

Windows deviates from the macOS paths on purpose: `git.exe` of Git for Windows finds `sh.exe`
and its helpers relative to its own `ucrt64/` and `usr/` folders, so MinGit stays unmodified
under `git/` (`git/cmd/git.exe`, `git/ucrt64/libexec/git-core/`, `git/usr/bin/`). Linux archives
have no Git: the Debian/Ubuntu package depends on the system one.

What Refract runs (`internal/tools`, #53) and with which environment:

| | Git it runs | Environment |
|---|---|---|
| macOS | `git/bin/git` | `GIT_EXEC_PATH=git/libexec/git-core`, `GIT_TEMPLATE_DIR=git/share/git-core/templates`, `GIT_CONFIG_NOSYSTEM=1` |
| Windows | `git\cmd\git.exe` (MinGit's launcher: it puts `ucrt64\bin` and `usr\bin` first on `PATH`) | `GIT_CONFIG_NOSYSTEM=1` only. No `GIT_EXEC_PATH` or `GIT_TEMPLATE_DIR`: Git finds `ucrt64\libexec\git-core` and its templates relative to itself |

HTTPS remotes (push and pull) work with only these: on macOS Git uses the system libcurl, which
checks servers against `/etc/ssl/cert.pem` (part of macOS; certificates added only to the
Keychain are not used); on Windows MinGit's default libcurl uses Schannel and the Windows
certificate store, so ignoring MinGit's `etc/gitconfig` (which sets only `core.autocrlf`,
`core.symlinks`, `credential.helper=manager` and similar) does not affect them. The smoke test
reads an HTTPS remote on both.

**Pins.** `packaging/tools.lock` is the only place with versions, sources and checksums. The
Dockerfile, `scripts/install-tex-tools.sh` (CI), `scripts/build-tools.sh` and the release all
read it; a test fails if any of them hardcodes a pin. Every download is verified against the
lock before use (`scripts/tools-common.sh`).

| Component | Source | Notes |
|---|---|---|
| Tectonic (MIT) | official release assets | musl static on Linux, MSVC static CRT on Windows |
| ripgrep (MIT or Unlicense) | official release assets | musl static on Linux |
| SyncTeX (MIT) | built from the pinned commit of jlaurens/synctex | zlib linked statically; Windows cross-built with llvm-mingw |
| zlib (Zlib) | pinned release tarball, compiled into SyncTeX | |
| Git, macOS (GPLv2, parts LGPL 2.1) | built from the official source tarball, `RUNTIME_PREFIX`, HTTPS through macOS's libcurl, Apple Common Crypto; no OpenSSL, expat, Perl, Python, Tcl or gettext | links only libSystem, libz, libiconv, libcurl and the CoreFoundation and CoreServices frameworks, which macOS provides; the smoke test fails if anything else is linked |
| Git, Windows (GPLv2 and the MSYS2/MinGW component licences) | MinGit from Git for Windows, unmodified | |

**Licence obligations.** Each archive carries the licence texts under `licenses/`. Git is GPLv2:
the macOS archive includes the exact source tarball (`licenses/git/source/`) and a source
offer; the Windows archive includes MinGit's `LICENSE.txt`, its component list and a written
offer pointing at the Git for Windows and MSYS2 sources. Tectonic's binary statically includes
many Rust crates and TeX components; its `LICENSE` points to their licences, and the crate list
is in the Tectonic source tag recorded in the manifest. Installers that ship the archive must
keep `licenses/` and the manifest and show them (for example in an About or Licences screen).
Review the Windows and Tectonic notices again when bumping a pin.

**Gaps.** `windows-arm64` has no tools archive: upstream Tectonic publishes no Windows arm64
build. The release attaches `refract-tools-<tag>-gaps.txt` listing it, and no Windows arm64
desktop installer may be advertised as complete until Tectonic runs there. The macOS amd64
tools are built and smoke-tested on an Intel runner; the Windows archive is cross-built on
Linux and only smoke-tested on a Windows amd64 runner. Those runners have a Git installed:
the smoke test strips `PATH` so the bundled runtime is what runs, which is close to but not
identical to a clean machine.

To build one locally (macOS arm64 example): `scripts/build-tools.sh darwin arm64 v0.0.0-dev dist`,
then extract it and run `scripts/smoke-tools.sh <folder> darwin`.

## Server deployments

Server deployments are a distinct path: `server/` (a systemd unit and
`prepare-bundle.sh` for the compile sandbox) and the Docker image, configured with
`REFRACT_*` variables as described in "Deploy" in the top-level README. Desktop and CLI
packages never include `server/` files or the server's system-wide unit.

## Release artifacts

The release workflow builds the six existing portable archives, including README
and LICENSE. `scripts/package-deb.sh` packages each Linux binary with `dpkg-deb`
(available on the Ubuntu runner). Today's CLI packages contain only `/usr/bin/refract` and
documentation, have no maintainer scripts or required dependencies, and do not
touch credentials when installed, upgraded or removed. The desktop Debian/Ubuntu
package adds the Git dependency and desktop entry under the policy above. Prerelease tags use Debian's
`~` ordering (`v0.1.2-rc1` becomes `0.1.2~rc1`).

`scripts/package-metadata.py` uses Python 3.11+ and its standard library to verify
the expected nested executable paths and calculate SHA-256 hashes from the actual
archives. It generates a macOS Homebrew formula and three WinGet manifests (version,
English locale and the x64/arm64 zip installers). They are release assets too:

```sh
python3 scripts/package-metadata.py v0.1.2 dist /tmp/refract-packages
```

`SHA256SUMS` covers all published files, including the packages and metadata.
The release waits for CLI smoke tests on macOS ARM64, Linux AMD64 and Windows AMD64;
the Linux test installs the actual `.deb`. The other architectures are cross-built.
OAuth sign-in itself remains an interactive user step and is not exercised by CI.

## Homebrew

The tap is [ponytojas/homebrew-tap](https://github.com/ponytojas/homebrew-tap).
Copy the generated `homebrew/refract.rb` to `Formula/refract.rb` in that repository.
It uses the existing macOS binaries; neither Go nor Node is needed to install.
Validate on macOS before pushing the tap update:

```sh
ruby -c Formula/refract.rb
brew install ponytojas/tap/refract
brew test ponytojas/tap/refract
```

For automatic updates after stable releases, configure these in the **Refract**
repository's GitHub Actions settings:

- Secret `HOMEBREW_TAP_TOKEN`: a fine-grained token restricted to
  `ponytojas/homebrew-tap`, with **Contents: read and write**. Enter it directly
  in GitHub's settings, not in a shell command or a committed file.
- Variable `HOMEBREW_TAP_ENABLED`: `true`, after adding the secret.

The default `GITHUB_TOKEN` cannot write to the separate tap. Without this setup,
releases still publish packages and metadata, and the tap can be updated manually.
Prereleases never automatically update the stable formula.

## WinGet

The release asset `refract-winget-manifests.tar.gz` contains the directory structure
expected by [microsoft/winget-pkgs](https://github.com/microsoft/winget-pkgs).
Extract it and, on Windows with WinGet installed, validate the version directory:

```powershell
winget validate --manifest manifests/p/ponytojas/Refract/0.1.2
```

Test the local manifest in a disposable Windows environment (enabling local
manifests may require an administrator terminal):

```powershell
winget settings --enable LocalManifestFiles
winget install --manifest manifests/p/ponytojas/Refract/0.1.2
refract version
refract login --help
winget uninstall --id ponytojas.Refract --exact
```

After validation, submit the generated version directory in a pull request to
`microsoft/winget-pkgs`. Microsoft's acceptance is required before users can run
`winget install --id ponytojas.Refract --exact` from the community source. Repeat
for each version; no automatic submission or approval is assumed. Use the stable
release manifests for the community package.

## Tests

```sh
python3 -m unittest discover -s scripts -p 'test_*.py' -v
```

The Debian payload test needs `dpkg-deb`; it runs in Linux CI and skips if the tool
is absent locally. `make test` also runs these tests. No tests open the OAuth
browser or read real credentials.

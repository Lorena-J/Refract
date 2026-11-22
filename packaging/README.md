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
  it needs, Tectonic, SyncTeX and ripgrep. Debian/Ubuntu packages do not bundle Git:
  they declare it as a package dependency (`Depends: git`) and use the system copy.
- Licences and notices, and the documentation.
- A desktop entry and icons for graphical installs.
- Versions, checksums and redistribution terms (licences) of every bundled tool must be
  recorded with the release. #55 implements that record.

CLI-only packages (Homebrew formula, WinGet, portable archives) contain only the binary
and documentation until the desktop installers replace them.

## What a package must not do

- Enable or start a service. A per-user background service is created only when the
  person asks for it (#57), never by an installer, maintainer script or Homebrew
  `service` block.
- Create keys, databases, projects or any data folder at install time. `refract local`
  creates them on first run, in per-user locations.
- Store data, the key or caches inside the application directory: they live in
  per-user locations outside it, so replacing the app's executables leaves them intact.
- Run as root after installation, or ship setuid or world-writable files.
- Touch an existing data folder or the stored credentials on install, upgrade or
  removal. Uninstalling the app keeps the person's data.
- Add server configuration (secrets, listening addresses or sign-in settings).

The packaging tests (`scripts/test_packaging.py`) enforce the rules that can be
checked: no maintainer scripts, no service in the Homebrew formula, and a payload that
contains only the allowed paths.

## Server deployments

Server deployments are a distinct path: `server/` (a systemd unit and
`prepare-bundle.sh` for the compile sandbox) and the Docker image, configured with
`REFRACT_*` variables as described in "Deploy" in the top-level README. Desktop and CLI
packages never include or enable them, and server files are never added to packages.

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

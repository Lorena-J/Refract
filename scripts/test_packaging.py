"""Packaging contracts: checksums, nested executables, Debian payload and metadata."""

import hashlib
import io
import os
import re
from pathlib import Path
import runpy
import shutil
import subprocess
import tarfile
import tempfile
import unittest
import zipfile


ROOT = Path(__file__).resolve().parents[1]
generate = runpy.run_path(str(ROOT / "scripts/package-metadata.py"))["generate"]


class PackagingTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.addCleanup(subprocess.call, ["chmod", "-R", "u+w", self.temp.name])
        self.archives = self.root / "dist"
        self.archives.mkdir()
        self.output = self.root / "metadata"

    def archives_for(self, tag):
        for platform in ("darwin", "windows"):
            for arch in ("amd64", "arm64"):
                name = f"refract-{tag}-{platform}-{arch}"
                if platform == "windows":
                    with zipfile.ZipFile(self.archives / f"{name}.zip", "w") as archive:
                        archive.writestr(f"{name}/refract.exe", f"{platform}-{arch}")
                else:
                    with tarfile.open(self.archives / f"{name}.tar.gz", "w:gz") as archive:
                        member = tarfile.TarInfo(f"{name}/refract")
                        member.mode = 0o755
                        member.size = len(arch)
                        archive.addfile(member, io.BytesIO(arch.encode()))

    def test_metadata_matches_release_archives(self):
        self.archives_for("v1.2.3")
        generate("v1.2.3", self.archives, self.output)
        formula = (self.output / "homebrew/refract.rb").read_text()
        self.assertIn("/releases/download/v1.2.3/", formula)
        self.assertNotIn('depends_on "', formula)
        winget = self.output / "winget/manifests/p/ponytojas/Refract/1.2.3"
        self.assertEqual(len(list(winget.glob("*.yaml"))), 3)
        installer = (winget / "ponytojas.Refract.installer.yaml").read_text()
        self.assertIn("NestedInstallerType: portable", installer)
        for platform in ("darwin", "windows"):
            for arch in ("amd64", "arm64"):
                ext = "zip" if platform == "windows" else "tar.gz"
                filename = f"refract-v1.2.3-{platform}-{arch}.{ext}"
                checksum = hashlib.sha256((self.archives / filename).read_bytes()).hexdigest()
                document = installer if platform == "windows" else formula
                self.assertIn(filename, document)
                self.assertIn(checksum.upper() if platform == "windows" else checksum, document)
                if platform == "windows":
                    self.assertIn(f"RelativeFilePath: refract-v1.2.3-windows-{arch}/refract.exe", installer)

    def test_rejects_missing_nested_executable_before_writing_metadata(self):
        self.archives_for("v1.2.3")
        with zipfile.ZipFile(self.archives / "refract-v1.2.3-windows-arm64.zip", "w") as archive:
            archive.writestr("refract.exe", "wrong path")
        with self.assertRaises(KeyError):
            generate("v1.2.3", self.archives, self.output)
        self.assertFalse(self.output.exists())

    def test_version_validation_and_prerelease(self):
        for tag in ("main", "v1.2.3/escape", 'v1.2.3"', "v1.2.3\n"):
            with self.assertRaises(ValueError):
                generate(tag, self.archives, self.output)
        self.archives_for("v1.2.3-rc1")
        generate("v1.2.3-rc1", self.archives, self.output)
        self.assertIn("/releases/download/v1.2.3-rc1/", (self.output / "homebrew/refract.rb").read_text())

    @unittest.skipUnless(shutil.which("dpkg-deb"), "Debian package verification needs dpkg-deb")
    def test_deb_installs_only_cli_and_docs_as_root(self):
        binary = self.root / "refract"
        binary.write_bytes(b"test executable\n")
        for arch in ("amd64", "arm64"):
            subprocess.run(["sh", str(ROOT / "scripts/package-deb.sh"), "v1.2.3-rc1", arch,
                            str(binary), str(self.archives)], cwd=ROOT, check=True, capture_output=True)
            package = self.archives / f"refract_1.2.3~rc1_{arch}.deb"
            control = subprocess.check_output(["dpkg-deb", "--field", str(package)], text=True)
            self.assertIn(f"Architecture: {arch}\n", control)
            self.assertIn("Version: 1.2.3~rc1\n", control)
            self.assertNotIn("Depends:", control)
            contents = subprocess.check_output(["dpkg-deb", "--fsys-tarfile", str(package)])
            with tarfile.open(fileobj=io.BytesIO(contents)) as archive:
                self.assert_archive_members(archive, {
                    ".": tarfile.DIRTYPE,
                    "./usr": tarfile.DIRTYPE,
                    "./usr/bin": tarfile.DIRTYPE,
                    "./usr/bin/refract": tarfile.REGTYPE,
                    "./usr/share": tarfile.DIRTYPE,
                    "./usr/share/doc": tarfile.DIRTYPE,
                    "./usr/share/doc/refract": tarfile.DIRTYPE,
                    "./usr/share/doc/refract/copyright": tarfile.REGTYPE,
                    "./usr/share/doc/refract/README.md": tarfile.REGTYPE,
                })
                self.assertEqual(archive.getmember("./usr/bin/refract").mode, 0o755)
                self.assertEqual(archive.extractfile("./usr/bin/refract").read(), binary.read_bytes())
            scripts = subprocess.check_output(["dpkg-deb", "--ctrl-tarfile", str(package)])
            with tarfile.open(fileobj=io.BytesIO(scripts)) as archive:
                self.assert_archive_members(archive, {
                    ".": tarfile.DIRTYPE,
                    "./control": tarfile.REGTYPE,
                })

    def assert_archive_members(self, archive, expected):
        members = archive.getmembers()
        self.assertCountEqual([(member.name, member.type) for member in members], expected.items())
        for member in members:
            self.assertEqual((member.uid, member.gid), (0, 0), member.name)
            # No setuid, setgid or world-writable entries.
            self.assertFalse(member.mode & 0o6002, member.name)

    def test_policy_documented_and_no_install_time_side_effects(self):
        policy = (ROOT / "packaging/README.md").read_text()
        for heading in ("## What a package may contain", "## What a package must not do",
                        "## Server deployments"):
            self.assertIn(heading, policy)
        self.assertIn("../libexec/refract/", policy)
        readme = (ROOT / "README.md").read_text()
        for stale in ("not a running server", "Do not add server", "does not start a server"):
            self.assertNotIn(stale, policy + readme)
        deb = (ROOT / "scripts/package-deb.sh").read_text()
        # The only control file the script may write is DEBIAN/control: no maintainer scripts.
        self.assertEqual(set(re.findall(r"DEBIAN/([\w.-]+)", deb)), {"control"})
        # Nothing enabled or installed system-wide, and no data or configuration paths.
        for marker in ("systemctl", "lib/systemd/system", "/etc/", "/var/"):
            self.assertNotIn(marker, deb)
        # Homebrew install-time hooks: a formula may declare a service, never run code at install.
        self.archives_for("v1.2.3")
        generate("v1.2.3", self.archives, self.output)
        for formula in (self.output / "homebrew").rglob("*.rb"):
            for marker in ("post_install", "postflight", "systemctl", "launchctl"):
                self.assertNotIn(marker, formula.read_text())

    @unittest.skipUnless(shutil.which("ruby"), "Formula install verification needs Ruby")
    def test_formula_install_only_installs_binary(self):
        self.archives_for("v1.2.3")
        generate("v1.2.3", self.archives, self.output)
        formula = self.output / "homebrew/refract.rb"
        for existing_data in (False, True):
            with self.subTest(existing_data=existing_data):
                sandbox = self.root / f"formula-{existing_data}"
                home, var, prefix, source = (sandbox / name for name in ("home", "var", "prefix", "source"))
                for directory in (home, var, prefix, source):
                    directory.mkdir(parents=True)
                (source / "refract").write_bytes(b"formula binary\n")
                (home / "keep").write_bytes(b"home data\n")
                if existing_data:
                    (var / "refract").mkdir()
                    (var / "refract/data").write_bytes(b"existing data\n")
                env = {**os.environ, "HOME": str(home), "TEST_VAR": str(var), "TEST_PREFIX": str(prefix)}
                result = subprocess.run(["ruby", str(ROOT / "scripts/formula_install_fixture.rb"), str(formula)],
                                        cwd=source, env=env, capture_output=True, text=True)
                self.assertEqual(result.returncode, 0, result.stderr)
                expected = {
                    "home": None, "home/keep": b"home data\n", "var": None,
                    "prefix": None, "prefix/bin": None, "prefix/bin/refract": b"formula binary\n",
                    "source": None, "source/refract": b"formula binary\n",
                }
                if existing_data:
                    expected.update({"var/refract": None, "var/refract/data": b"existing data\n"})
                actual = {}
                for path in sandbox.rglob("*"):
                    self.assertFalse(path.is_symlink(), path)
                    self.assertTrue(path.is_dir() or path.is_file(), path)
                    actual[path.relative_to(sandbox).as_posix()] = None if path.is_dir() else path.read_bytes()
                self.assertEqual(actual, expected)

    def prepare_bundle(self, dest):
        """Runs prepare-bundle.sh with a fake curl (serves a tiny bundle) and tectonic."""
        fake = self.root / "fakebin"
        if not fake.exists():
            fake.mkdir()
            digest = "6ffe055852f8faf66c0acbe1a7fb27f87b869a90bad1204f3bf4d9683f597c7c"
            tar = self.root / "bundle.tar"
            with tarfile.open(tar, "w") as archive:
                data = digest.encode()
                member = tarfile.TarInfo("SHA256SUM")
                member.size = len(data)
                archive.addfile(member, io.BytesIO(data))
            (fake / "curl").write_text(
                '#!/bin/sh\nwhile [ $# -gt 0 ]; do [ "$1" = -o ] && out=$2; shift; done\ncp "%s" "$out"\n' % tar
            )
            (fake / "tectonic").write_text(
                '#!/bin/sh\nmkdir -p "$TECTONIC_CACHE_DIR/formats" && echo fmt > "$TECTONIC_CACHE_DIR/formats/new.fmt"\n'
            )
            for name in ("curl", "tectonic"):
                (fake / name).chmod(0o755)
        env = {**os.environ, "PATH": f"{fake}:{os.environ['PATH']}"}
        return subprocess.run(
            ["sh", str(ROOT / "packaging/server/prepare-bundle.sh"), str(dest)],
            env=env, capture_output=True, text=True,
        )

    def test_prepare_bundle_replaces_both_or_refuses(self):
        dest = self.root / "server"
        first = self.prepare_bundle(dest)
        self.assertEqual(first.returncode, 0, first.stderr)
        self.assertTrue((dest / "cache/formats/new.fmt").exists())
        # Rerunning with either folder left over refuses and changes nothing.
        for keep in ("bundle", "cache"):
            subprocess.check_call(["chmod", "-R", "u+w", str(dest)])
            shutil.rmtree(dest)
            (dest / keep).mkdir(parents=True)
            again = self.prepare_bundle(dest)
            self.assertNotEqual(again.returncode, 0)
            self.assertIn("remove both", again.stderr)
            self.assertEqual(sorted(p.name for p in dest.iterdir()), [keep])
            self.assertEqual(list((dest / keep).iterdir()), [])
        # Following the printed instructions works, and nothing nests.
        subprocess.check_call(["chmod", "-R", "u+w", str(dest)])
        shutil.rmtree(dest)
        self.assertEqual(self.prepare_bundle(dest).returncode, 0)
        self.assertTrue((dest / "cache/formats/new.fmt").exists())
        self.assertFalse((dest / "cache/cache").exists())


if __name__ == "__main__":
    unittest.main()

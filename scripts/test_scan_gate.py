import datetime as dt
import json
import os
import tempfile
import unittest

import scan_gate

TODAY = dt.date(2026, 11, 19)
DOCKERFILE = """
ARG TECTONIC_VERSION=0.17.0
      amd64) triple=x86_64-unknown-linux-musl  sha=8533d07f9ccbd7a65824b9e0459041bca34af1eb33daba48f59215593753a3b7 ;; \\
      arm64) triple=aarch64-unknown-linux-musl sha=b10954a95404f3ab2328d2fa59a5ebab8e657f893fab096f98be8db7c0c979b8 ;; \\
ARG SYNCTEX_COMMIT=c3641da45b41cd61f9e625fe722d0a8d54cedaa9
"""
SYFT = {"artifacts": [{"name": "git"}, {"name": "busybox"}]}


def match(vid, sev, pkg, related=()):
    return {
        "vulnerability": {"id": vid, "severity": sev, "fix": {"versions": ["9.9"], "state": "fixed"}},
        "relatedVulnerabilities": [{"id": r} for r in related],
        "artifact": {"name": pkg, "version": "1.0", "type": "apk"},
    }


def grype(*matches):
    return {"matches": list(matches), "descriptor": {"version": "0.121.1", "db": {"status": {"built": "2026-11-19T00:00:00Z"}}}}


def exception(**over):
    e = {"id": "CVE-2026-1", "package": "git", "reason": "not reachable", "reviewed_by": "someone",
         "reviewed_on": "2026-11-10", "expires": "2026-12-10"}
    e.update(over)
    return {"exceptions": [e]}


def run(g, exceptions={"exceptions": []}):
    return scan_gate.gate(g, SYFT, DOCKERFILE, exceptions, "ghcr.io/x/refract@sha256:abc", {"tectonic": "Tectonic 0.17.0"}, TODAY)


class GateTest(unittest.TestCase):
    def test_clean_image_publishes(self):
        blocked, report = run(grype())
        self.assertFalse(blocked)
        self.assertIn("publishable", report)

    def test_only_critical_blocks(self):
        blocked, report = run(grype(match("CVE-2026-2", "High", "git"), match("CVE-2026-3", "Medium", "busybox")))
        self.assertFalse(blocked)
        self.assertIn("CVE-2026-2", report)  # listed, not blocking
        blocked, report = run(grype(match("CVE-2026-1", "Critical", "git")))
        self.assertTrue(blocked)
        self.assertIn("## Blocking", report)

    def test_reviewed_exception_waives(self):
        blocked, report = run(grype(match("CVE-2026-1", "Critical", "git")), exception())
        self.assertFalse(blocked)
        self.assertIn("not reachable", report)
        # An alias (GHSA for a CVE) matches too; another package doesn't.
        blocked, _ = run(grype(match("GHSA-xxxx", "Critical", "git", related=["CVE-2026-1"])), exception())
        self.assertFalse(blocked)
        blocked, _ = run(grype(match("CVE-2026-1", "Critical", "busybox")), exception())
        self.assertTrue(blocked)

    def test_bad_exceptions_block(self):
        for bad in (exception(expires="2026-11-18"), exception(reason=""), exception(expires="2027-06-01"), exception(reviewed_on="soon")):
            blocked, report = run(grype(), bad)
            self.assertTrue(blocked, bad)
            self.assertIn("need attention", report)

    def test_native_tools_inventory(self):
        _, report = run(grype())
        self.assertIn("| tectonic | `0.17.0` | Tectonic 0.17.0 | no | amd64 `8533d07f9ccb…`, arm64 `b10954a95404…`", report)
        self.assertIn("| synctex | `c3641da45b41cd61f9e625fe722d0a8d54cedaa9` | not run | no | built from the pinned commit", report)

    def test_cli_exit_codes(self):
        with tempfile.TemporaryDirectory() as d:
            def write(name, data):
                p = os.path.join(d, name)
                with open(p, "w") as f:
                    f.write(data if isinstance(data, str) else json.dumps(data))
                return p
            args = ["--syft", write("s.json", SYFT), "--dockerfile", write("Dockerfile", DOCKERFILE),
                    "--exceptions", write("e.json", {"exceptions": []}), "--image", "img",
                    "--report", os.path.join(d, "report.md"), "--today", "2026-11-19"]
            self.assertEqual(scan_gate.main(["--grype", write("g.json", grype())] + args), 0)
            self.assertEqual(scan_gate.main(["--grype", write("g.json", grype(match("CVE-1", "Critical", "git")))] + args), 1)
            self.assertEqual(scan_gate.main(["--grype", write("g.json", "{}")] + args), 2)
            self.assertEqual(scan_gate.main(["--grype", write("g.json", "not json")] + args), 2)


if __name__ == "__main__":
    unittest.main()

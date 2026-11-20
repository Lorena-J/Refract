#!/usr/bin/env python3
"""Decides whether a scanned image may be published, and writes the report.

Inputs come from scripts/scan-image.sh: Grype's JSON (the vulnerabilities
found), Syft's JSON (what the image holds), and the versions the image's
native tools report. A finding of Critical severity blocks publication
unless security/image-scan-exceptions.json holds a reviewed, unexpired
exception for it. Every other finding is listed in the report and doesn't
block. An expired or incomplete exception fails the gate too, so that each
one is reviewed again.

Usage: scan_gate.py --grype G --syft S --dockerfile D --exceptions E
                    --image REF --tool name=reported-version ... --report OUT
Exit status: 0 publish, 1 blocked, 2 bad input.
"""

import argparse
import datetime as dt
import json
import re
import sys

BLOCKING = "Critical"
MAX_EXCEPTION_DAYS = 90
EXCEPTION_FIELDS = ("id", "package", "reason", "reviewed_by", "reviewed_on", "expires")


class InputError(Exception):
    pass


def load_exceptions(data, today):
    """Returns the usable exceptions and the problems found in the file."""
    usable, problems = [], []
    for i, e in enumerate(data.get("exceptions", [])):
        where = f"exception {i + 1} ({e.get('id', '?')})"
        missing = [f for f in EXCEPTION_FIELDS if not str(e.get(f, "")).strip()]
        if missing:
            problems.append(f"{where}: missing {', '.join(missing)}")
            continue
        try:
            reviewed = dt.date.fromisoformat(e["reviewed_on"])
            expires = dt.date.fromisoformat(e["expires"])
        except ValueError:
            problems.append(f"{where}: dates must be YYYY-MM-DD")
            continue
        if expires < today:
            problems.append(f"{where}: expired on {expires}; review it again")
            continue
        if (expires - reviewed).days > MAX_EXCEPTION_DAYS:
            problems.append(f"{where}: expires more than {MAX_EXCEPTION_DAYS} days after its review")
            continue
        usable.append(e)
    return usable, problems


def ids_of(match):
    v = match.get("vulnerability", {})
    ids = {v.get("id", "")}
    ids.update(r.get("id", "") for r in match.get("relatedVulnerabilities", []) or [])
    return {i for i in ids if i}


def excepted(match, exceptions):
    pkg = match.get("artifact", {}).get("name", "")
    ids = ids_of(match)
    for e in exceptions:
        if e["id"] in ids and e["package"] == pkg:
            return e
    return None


def dockerfile_pins(text):
    def arg(name):
        m = re.search(rf"^ARG {name}=(\S+)", text, re.M)
        return m.group(1) if m else ""

    shas = dict(re.findall(r"^\s+(amd64|arm64)\).*?sha=([0-9a-f]{64})", text, re.M))
    return {
        "tectonic": {
            "pin": arg("TECTONIC_VERSION"),
            "sha256": shas,
            "source": "https://github.com/tectonic-typesetting/tectonic/releases",
            "advisories": "https://github.com/tectonic-typesetting/tectonic/security/advisories and the crates in its Cargo.lock (https://rustsec.org)",
        },
        "synctex": {
            "pin": arg("SYNCTEX_COMMIT"),
            "sha256": {},
            "source": "https://github.com/jlaurens/synctex (built from source, statically linked with Alpine's zlib)",
            "advisories": "TeX Live's synctex changes (https://tug.org/texlive/) and zlib advisories for the Alpine version in the SBOM",
        },
    }


def gate(grype, syft, dockerfile, exceptions_data, image, tools, today):
    """Returns (blocked, report markdown)."""
    if "matches" not in grype:
        raise InputError("the Grype report has no matches list")
    exceptions, problems = load_exceptions(exceptions_data, today)
    blocking, waived, others = [], [], []
    for m in grype["matches"]:
        sev = m.get("vulnerability", {}).get("severity", "Unknown")
        if sev != BLOCKING:
            others.append(m)
        elif e := excepted(m, exceptions):
            waived.append((m, e))
        else:
            blocking.append(m)

    catalogued = {a.get("name", "") for a in syft.get("artifacts", [])}
    pins = dockerfile_pins(dockerfile)
    desc = grype.get("descriptor", {})
    db = desc.get("db", {}) or {}
    db_built = (db.get("status", {}) or {}).get("built") or db.get("built", "unknown")
    counts = {}
    for m in grype["matches"]:
        sev = m.get("vulnerability", {}).get("severity", "Unknown")
        counts[sev] = counts.get(sev, 0) + 1
    blocked = bool(blocking or problems)

    def row(m):
        v, a = m.get("vulnerability", {}), m.get("artifact", {})
        fix = ", ".join((v.get("fix") or {}).get("versions") or []) or (v.get("fix") or {}).get("state", "")
        return f"| {v.get('id', '')} | {v.get('severity', '')} | {a.get('name', '')} {a.get('version', '')} | {a.get('type', '')} | {fix} |"

    out = [
        f"# Image scan: {'BLOCKED' if blocked else 'publishable'}",
        "",
        f"- Image: `{image}`",
        f"- Scanner: Grype {desc.get('version', '?')}, database built {db_built}; SBOM by Syft",
        f"- Scanned on: {today}",
        f"- Policy: findings of {BLOCKING} severity block publication unless security/image-scan-exceptions.json has a reviewed, unexpired exception (at most {MAX_EXCEPTION_DAYS} days).",
        f"- Findings by severity: {', '.join(f'{k} {v}' for k, v in sorted(counts.items())) or 'none'}",
        "",
    ]
    if problems:
        out += ["## Exceptions that need attention", ""] + [f"- {p}" for p in problems] + [""]
    head = ["| Vulnerability | Severity | Package | Type | Fixed in |", "|---|---|---|---|---|"]
    if blocking:
        out += ["## Blocking", ""] + head + [row(m) for m in blocking] + [""]
    if waived:
        out += ["## Critical, with a reviewed exception", "", "| Vulnerability | Package | Reason | Reviewed by | Expires |", "|---|---|---|---|---|"]
        out += [f"| {m['vulnerability']['id']} | {e['package']} | {e['reason']} | {e['reviewed_by']} on {e['reviewed_on']} | {e['expires']} |" for m, e in waived] + [""]
    if others:
        out += ["## Other findings (not blocking)", ""] + head + [row(m) for m in others] + [""]
    out += [
        "## Native tools the scanner may not identify",
        "",
        "Built or downloaded outside the package manager, so Grype only knows them if Syft recognises the binary. Check their advisories by hand when they change and before each release.",
        "",
        "| Tool | Pinned | Reported by the image | Recognised by Syft | Checksums | Advisories |",
        "|---|---|---|---|---|---|",
    ]
    for name, p in pins.items():
        sums = ", ".join(f"{k} `{v[:12]}…`" for k, v in sorted(p["sha256"].items())) or "built from the pinned commit"
        out.append(f"| {name} | `{p['pin']}` | {tools.get(name, 'not run')} | {'yes' if name in catalogued else 'no'} | {sums} | {p['advisories']} |")
    out += ["", f"Sources: tectonic from {pins['tectonic']['source']}; synctex from {pins['synctex']['source']}.", ""]
    return blocked, "\n".join(out)


def main(argv=None):
    ap = argparse.ArgumentParser()
    for a in ("--grype", "--syft", "--dockerfile", "--exceptions", "--image", "--report"):
        ap.add_argument(a, required=True)
    ap.add_argument("--tool", action="append", default=[], help="name=version the image reports")
    ap.add_argument("--today", help="YYYY-MM-DD, for tests")
    args = ap.parse_args(argv)
    try:
        with open(args.grype) as f:
            grype = json.load(f)
        with open(args.syft) as f:
            syft = json.load(f)
        with open(args.dockerfile) as f:
            dockerfile = f.read()
        with open(args.exceptions) as f:
            exceptions = json.load(f)
        tools = dict(t.split("=", 1) for t in args.tool)
        today = dt.date.fromisoformat(args.today) if args.today else dt.date.today()
        blocked, report = gate(grype, syft, dockerfile, exceptions, args.image, tools, today)
    except (OSError, ValueError, InputError) as e:
        print(f"scan gate: {e}", file=sys.stderr)
        return 2
    with open(args.report, "w") as f:
        f.write(report)
    print(report.split("\n", 1)[0])
    return 1 if blocked else 0


if __name__ == "__main__":
    sys.exit(main())

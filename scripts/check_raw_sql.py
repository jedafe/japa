#!/usr/bin/env python3
"""Raw-SQL ratchet: fail if inline pgx call sites grow beyond the baseline.

The codebase has legacy inline SQL outside sqlc (db/queries/*.sql -> make
sqlc-gen). Removing it wholesale is a large refactor; this ratchet prevents
the count from GROWING — new queries must go through sqlc (Rule 2 pattern).

Baseline file: scripts/raw_sql_baseline.txt (one integer = max allowed sites).
Update the baseline DOWNWARD as sites are migrated; never raise it.

Excluded by design: advisory-lock EXECs, migrations, generated code, tests,
and the data-import layer (dynamic INSERT by column map, cannot be sqlc).

Usage: python3 scripts/check_raw_sql.py   # exit 1 if count > baseline
"""
import re
import subprocess
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
BASELINE = REPO / "scripts" / "raw_sql_baseline.txt"

# Direct DB call sites carrying inline SQL text (backtick or quoted strings
# starting with a keyword), plus CollectRows over raw strings.
CALL = re.compile(
    r'\.(Query|QueryRow|Exec|ExecBatch|CollectRows)\(\s*ctx\s*,\s*[(`]'
    r'\s*(SELECT|INSERT|UPDATE|DELETE|WITH|CREATE|ALTER|DROP|TRUNCATE)',
    re.IGNORECASE,
)
EXCLUDE_DIRS = {"internal/db/gen", "internal/db/migrations", "internal/migrate", "internal/dataimport"}
EXCLUDE_SUFFIX = "_test.go"


def iter_sites():
    out = subprocess.run(
        ["git", "ls-files", "internal/"], capture_output=True, text=True, cwd=REPO
    )
    for rel in out.stdout.splitlines():
        if rel.endswith(EXCLUDE_SUFFIX) or not rel.endswith(".go"):
            continue
        if any(rel.startswith(d + "/") for d in EXCLUDE_DIRS):
            continue
        p = REPO / rel
        for i, line in enumerate(p.read_text(errors="replace").splitlines(), 1):
            if "pg_advisory_xact_lock" in line:
                continue
            if CALL.search(line):
                yield f"{rel}:{i}"


def main():
    sites = sorted(iter_sites())
    baseline = int(BASELINE.read_text().strip()) if BASELINE.exists() else 0

    print(f"raw-SQL call sites: {len(sites)} (baseline {baseline})")

    if len(sites) > baseline:
        print("\nnew inline SQL detected — add the query to db/queries/*.sql and run `make sqlc-gen` instead:")
        for s in sites[:40]:
            print(f"  {s}")
        sys.exit(1)

    print("OK")


if __name__ == "__main__":
    main()

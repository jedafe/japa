#!/usr/bin/env python3
"""Extract all public routes from the JEOS route catalog.

Outputs a JSON list of {method, path} for every catalog entry whose line
contains the [public] tag. Paths are normalized:
  - /api/v1 prefix is stripped
  - :param is converted to {param}
  - duplicate methods from GET/POST splits are expanded
"""
import json
import re
import sys

CATALOG = ".jeos/conventions/api-routes-catalog.md"


def normalize_path(p: str) -> str:
    p = p.replace("/api/v1", "", 1)
    p = re.sub(r":(\w+)", r"{\1}", p)
    return p.rstrip("/") or "/"


def main():
    routes = []
    with open(CATALOG) as f:
        for line in f:
            if "[public" not in line:
                continue
            m = re.match(
                r"\s*(GET|POST|PUT|PATCH|DELETE|GET/POST|GET/PATCH)\s+(/\S+)",
                line,
            )
            if not m:
                continue
            path = normalize_path(m.group(2).split("?")[0])
            for meth in m.group(1).split("/"):
                routes.append({"method": meth, "path": path})

    # Deduplicate while preserving order
    seen = set()
    unique = []
    for r in routes:
        key = (r["method"], r["path"])
        if key not in seen:
            seen.add(key)
            unique.append(r)

    print(json.dumps(unique, indent=2))
    sys.stderr.write(f"Extracted {len(unique)} unique public routes\n")


if __name__ == "__main__":
    main()

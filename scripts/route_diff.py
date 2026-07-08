#!/usr/bin/env python3
"""Route reconciliation harness: diff server.go routes against the JEOS catalog.

Single source of truth for "code == catalog". Normalizes Chi {x} and catalog :x
param syntax, accounts for the /api/v1 base and the nested /auth sub-router, and
prints catalog-only (implement/rename-target) and code-only (document/rename-
source) route sets.

Usage: python3 scripts/route_diff.py [--raw]
Exit 0 always; intended for human reading + CI drift reporting.
"""
import re
import sys

SERVER = "internal/server/server.go"
CATALOG = ".jeos/conventions/api-routes-catalog.md"

# Root-path infra routes that intentionally live outside /api/v1 and outside the
# catalog's per-domain sections (SEO/OG/RSS are catalogued separately at root).
INFRA_PREFIXES = ("/health", "/ready")


def collapse_params(p: str) -> str:
    p = re.sub(r"\{(\w+)\}", r":\1", p)      # chi {x} -> :x
    p = re.sub(r":\w+", ":x", p)             # collapse param NAMES for structural compare
    return p.rstrip("/") or "/"


def parse_server(path: str):
    """Return set of (METHOD, normalized_path).

    Tracks the /auth sub-route so its relative registrations get the /auth
    prefix restored. Everything else is relative to /api/v1, which the catalog
    also strips, so no prefix is added.
    """
    routes = set()
    auth_depth = None
    depth = 0
    with open(path) as f:
        for line in f:
            depth += line.count("{") - line.count("}")
            if re.search(r'\.Route\("/auth"', line):
                auth_depth = depth  # inside this brace depth we are under /auth
            if auth_depth is not None and depth < auth_depth:
                auth_depth = None
            # Match both `r.Get("…")` and the continuation-line form where the
            # method sits on its own indented line after `r.With(...).`:
            #     r.With(mw...).
            #         Post("/path", handler)
            # The (?<![A-Za-z]) lookbehind keeps `Post(` from matching inside an
            # identifier like `RepostGet(` while allowing `.Post(` and `  Post(`.
            m = re.search(r'(?<![A-Za-z])(Get|Post|Put|Patch|Delete)\("(/[^"]*)"', line)
            if not m:
                continue
            meth, rpath = m.group(1).upper(), m.group(2)
            if auth_depth is not None and not rpath.startswith("/auth"):
                rpath = "/auth" + rpath
            routes.add((meth, collapse_params(rpath)))
    return routes


def parse_catalog(path: str):
    routes = set()
    with open(path) as f:
        for line in f:
            m = re.match(r"\s*(GET|POST|PUT|PATCH|DELETE|GET/POST|GET/PATCH)\s+(/\S+)", line)
            if not m:
                continue
            path_ = m.group(2).replace("/api/v1", "")
            for meth in m.group(1).split("/"):
                routes.add((meth, collapse_params(path_)))
    return routes


def main():
    code = parse_server(SERVER)
    cat = parse_catalog(CATALOG)
    code = {r for r in code if not r[1].startswith(INFRA_PREFIXES)}

    only_cat = sorted(cat - code)
    only_code = sorted(code - cat)
    print(f"MATCH={len(code & cat)}  CATALOG_ONLY={len(only_cat)}  CODE_ONLY={len(only_code)}")
    print("\n== CATALOG-ONLY (implement or rename-target) ==")
    for meth, p in only_cat:
        print(f"  {meth:6} {p}")
    print("\n== CODE-ONLY (document or rename-source) ==")
    for meth, p in only_code:
        print(f"  {meth:6} {p}")
    # Reconciliation is complete when only_cat and only_code are both empty
    # (modulo intentionally-uncatalogued infra). CI can assert on these counts.


if __name__ == "__main__":
    main()

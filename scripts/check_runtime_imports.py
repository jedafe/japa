#!/usr/bin/env python3
"""Assert every bare import in an app's build output is a declared dependency.

adapter-node externalizes each app's `dependencies` — they stay as bare
imports in build/ and must resolve from node_modules at runtime. With bun's
isolated node_modules layout, a bare import only resolves if it is a direct
dependency of the app; transitives (e.g. memoize via layerchart) are NOT
reachable from build chunks, and --production installs drop devDeps entirely.

This catches both failure modes before deploy: undeclared externals and
transitives that slipped into the bundle as bare imports.

Usage: python3 scripts/check_runtime_imports.py <app-dir> [<app-dir>...]
Exits 0 if every external import is declared, 1 otherwise. Apps without a
build/ directory are skipped (SPA/static builds have no server bundle).
"""

import json
import os
import re
import sys

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

# Strip block comments so JSDoc `import('x').Type` annotations are not
# mistaken for real imports, then match statement-level imports and dynamic
# import() calls. Line comments are deliberately NOT stripped: minified
# chunks are single-line, so a `//` inside a string literal would swallow
# every real import that follows it on that line.
BLOCK_COMMENT = re.compile(r"/\*.*?\*/", re.S)
IMPORT_FROM = re.compile(
    r"""import\s+(?:[\w${}*\s,]+\s+from\s+)?['"]([^'"]+)['"]"""
)
IMPORT_CALL = re.compile(r"""import\s*\(\s*['"]([^'"]+)['"]\s*\)""")
EXPORT_FROM = re.compile(r"""export\s+[\w${}*\s,]+\s+from\s+['"]([^'"]+)['"]""")


def package_root(spec: str) -> str:
    """'@scope/name/sub' -> '@scope/name'; 'name/sub' -> 'name'."""
    parts = spec.split("/")
    return "/".join(parts[:2]) if spec.startswith("@") else parts[0]


def declared_deps(app_dir: str) -> set:
    """Direct dependency names visible to the app's build output."""
    deps = set()
    for path in [
        os.path.join(app_dir, "package.json"),
        os.path.join(REPO, "package.json"),
    ]:
        try:
            pkg = json.load(open(path))
        except FileNotFoundError:
            continue
        deps.update(pkg.get("dependencies", {}))
    return deps


def collect_imports(build_dir: str) -> set:
    found = set()
    for root, _, files in os.walk(build_dir):
        for name in files:
            if not name.endswith(".js"):
                continue
            src = open(os.path.join(root, name), encoding="utf-8", errors="replace").read()
            src = BLOCK_COMMENT.sub("", src)
            for spec in IMPORT_FROM.findall(src) + EXPORT_FROM.findall(src) + IMPORT_CALL.findall(src):
                if spec.startswith((".", "/", "node:", "#", "$")):
                    continue
                found.add(package_root(spec))
    return found


def main() -> int:
    failures = False
    for rel in sys.argv[1:]:
        app_dir = os.path.join(REPO, rel)
        build_dir = os.path.join(app_dir, "build")
        if not os.path.isdir(build_dir):
            print(f"{rel}: no build/ — skipped")
            continue

        undeclared = collect_imports(build_dir) - declared_deps(app_dir)
        if undeclared:
            failures = True
            for name in sorted(undeclared):
                print(f"{rel}: ERROR external import '{name}' is not in {rel}/package.json dependencies")
            print(f"  fix: add the missing package(s) to {rel}/package.json dependencies")
        else:
            print(f"{rel}: OK")
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())

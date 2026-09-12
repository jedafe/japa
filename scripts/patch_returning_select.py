#!/usr/bin/env python3
"""
Patch the current de-tenanted query files so that single-table SELECT and
RETURNING column lists include tenant_id in the same position as the original
(pre-de-tenant) queries. This restores the model-matching row shape for sqlc.
Only the column list is patched; predicates, values, and parameters are left
unchanged.
"""
import re
import subprocess
import sys
from pathlib import Path

QUERIES_DIR = Path("internal/db/queries")
MODELS_FILE = Path("internal/db/gen/models.go")
ORIGIN = "1f94e11c"


def get_original(path: Path) -> str:
    out = subprocess.run(
        ["git", "show", f"{ORIGIN}:{path}"],
        capture_output=True,
        text=True,
        check=True,
    )
    return out.stdout


def parse_models() -> set[tuple[str, ...]]:
    """Return a set of full table column lists from generated models."""
    text = MODELS_FILE.read_text()
    columns: set[tuple[str, ...]] = set()
    for m in re.finditer(r"^type\s+(\w+)\s+struct\s*\{(.*?)\n\}", text, re.M | re.S):
        body = m.group(2)
        cols = []
        for line in body.splitlines():
            tag = re.search(r'`json:"([^"]+)"`', line)
            if tag:
                cols.append(tag.group(1))
        if cols:
            columns.add(tuple(cols))
    return columns


MODEL_COLUMNS = parse_models()


def split_statements(text: str) -> list[tuple[str, str]]:
    """Split SQL text into an ordered list of (query name, statement body)."""
    pattern = re.compile(r"(?m)^-- name: (\S+)", re.M)
    matches = list(pattern.finditer(text))
    stmts = []
    for i, m in enumerate(matches):
        name = m.group(1)
        start = m.start()
        end = matches[i + 1].start() if i + 1 < len(matches) else len(text)
        stmts.append((name, text[start:end]))
    return stmts


def extract_lists(stmt: str) -> dict[str, tuple[int, int, str]]:
    """Find SELECT and RETURNING column lists and return {kind: (start, end, cols)}."""
    found = {}

    # SELECT ... FROM (top-level, not in subquery). We take the first SELECT.
    for m in re.finditer(r"(?m)^([ \t]*SELECT[ \t]+)([\s\S]+?)([ \t]*\n[ \t]*FROM[ \t]+\w+)", stmt):
        prefix, raw, suffix = m.group(1), m.group(2), m.group(3)
        # Skip if this SELECT is inside parentheses (subquery in WHERE/EXISTS)
        prefix_text = stmt[:m.start()]
        opens = prefix_text.count("(")
        closes = prefix_text.count(")")
        if opens > closes:
            continue
        found["SELECT"] = (m.start(2), m.end(2), raw)
        break

    # RETURNING ... ;
    for m in re.finditer(r"(RETURNING[ \t]+)([\s\S]+?)([ \t]*;)", stmt):
        found["RETURNING"] = (m.start(2), m.end(2), m.group(2))
        break

    return found


def normalize_list(raw: str) -> list[str]:
    # Remove line continuations and normalize whitespace
    text = re.sub(r"\s+", " ", raw.replace("\n", " "))
    cols = [c.strip() for c in text.split(",") if c.strip()]
    # Strip table alias and cast/aliases for comparison
    out = []
    for c in cols:
        c = re.sub(r"\b\w+\.", "", c)
        c = re.sub(r"::\w+(?:\([^)]*\))?", "", c)
        c = re.sub(r"\s+AS\s+\w+", "", c, flags=re.I)
        c = re.sub(r"sqlc\.narg\((\w+)\)", r"\1", c, flags=re.I)
        c = re.sub(r"\(\)", "", c)
        c = c.strip()
        if c:
            out.append(c)
    return out


def patch_statement(current: str, original: str) -> tuple[str, bool]:
    current_lists = extract_lists(current)
    original_lists = extract_lists(original)
    changed = False
    new_current = current
    offset = 0
    for kind in ("SELECT", "RETURNING"):
        if kind not in current_lists or kind not in original_lists:
            continue
        c_start, c_end, c_raw = current_lists[kind]
        o_start, o_end, o_raw = original_lists[kind]
        c_norm = normalize_list(c_raw)
        o_norm = normalize_list(o_raw)
        if "tenant_id" in c_norm:
            continue
        if "tenant_id" not in o_norm:
            continue
        # Check that current list equals original list with tenant_id removed
        o_without = [c for c in o_norm if c != "tenant_id"]
        if c_norm != o_without:
            continue
        # Only patch if the original list is a full table model column list
        if tuple(o_norm) not in MODEL_COLUMNS:
            continue
        # Replace current raw list with original raw list
        changed = True
        # Adjust positions because we are replacing in a string that may have been patched
        c_start += offset
        c_end += offset
        new_current = new_current[:c_start] + o_raw + new_current[c_end:]
        offset += len(o_raw) - (c_end - c_start)
    return new_current, changed


def fix_file(path: Path) -> bool:
    current = path.read_text()
    try:
        original = get_original(path)
    except subprocess.CalledProcessError:
        return False

    current_stmts = split_statements(current)
    original_stmts = dict(split_statements(original))

    changed = False
    new_parts = []
    for name, stmt in current_stmts:
        if name in original_stmts:
            new_stmt, c = patch_statement(stmt, original_stmts[name])
            if c:
                changed = True
            new_parts.append(new_stmt)
        else:
            new_parts.append(stmt)
    if changed:
        path.write_text("".join(new_parts))
        print(f"patched {path}")
    return changed


def main():
    changed = False
    for sql in sorted(QUERIES_DIR.glob("*.sql")):
        if fix_file(sql):
            changed = True
    return 0


if __name__ == "__main__":
    sys.exit(main())

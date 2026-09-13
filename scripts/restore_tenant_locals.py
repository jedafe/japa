#!/usr/bin/env python3
"""
Restore `tenantID` in front of blanked requireTenant calls when the variable
is actually used later in the same function.
"""
from pathlib import Path
import re

TARGETS = [
    Path("internal/digest/handlers.go"),
    Path("internal/remittance/handlers.go"),
    Path("internal/travel/handlers.go"),
    Path("internal/spaces/space_admin_handlers.go"),
    Path("internal/spaces/space_types_handlers.go"),
    Path("internal/spaces/space_hierarchy_handlers.go"),
]

FUNC_START = re.compile(r"^func (?:\([^\)]+\) )?\w+\(" )


def find_next_func(lines: list[str], start: int) -> int:
    for i in range(start + 1, len(lines)):
        if FUNC_START.match(lines[i]):
            return i
    return len(lines)


def tenant_used_later(lines: list[str], current: int, end: int) -> bool:
    return any(re.search(r"\btenantID\b", line) for line in lines[current+1:end])


def fix_file(path: Path):
    text = path.read_text()
    lines = text.splitlines(keepends=True)
    out = list(lines)
    changed = False

    i = 0
    while i < len(out):
        if FUNC_START.match(out[i]):
            func_end = find_next_func(out, i)
            j = i + 1
            while j < func_end:
                line = out[j]
                # Match blanked requireTenant calls introduced earlier
                m = re.match(r"^(\s*)_,\s*(ok|_)(?:\s*(:=|=))\s*(h?\.?requireTenant\(w,\s*r\))$", line)
                if m and tenant_used_later(out, j, func_end):
                    indent, rhs = m.group(1), m.group(4)
                    out[j] = f"{indent}tenantID, ok := {rhs}\n"
                    changed = True
                j += 1
            i = func_end
        else:
            i += 1

    if changed:
        path.write_text("".join(out))
        print(f"patched {path}")


if __name__ == "__main__":
    for path in TARGETS:
        if path.exists():
            fix_file(path)

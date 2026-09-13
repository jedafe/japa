#!/usr/bin/env python3
"""Blank out unused tenantID/org/tenantPart locals, preserving ok redeclaration rules."""
from pathlib import Path
import re

TARGETS = [
    Path("internal/affiliate/handlers.go"),
    Path("internal/wallet/currency_handlers.go"),
    Path("internal/payments/handlers.go"),
    Path("internal/coupons/handlers.go"),
    Path("internal/digest/handlers.go"),
    Path("internal/remittance/handlers.go"),
    Path("internal/travel/handlers.go"),
    Path("internal/travel/service.go"),
    Path("internal/users/handlers.go"),
    Path("internal/customfields/handlers.go"),
    Path("internal/spaces/space_admin_handlers.go"),
    Path("internal/spaces/space_types_handlers.go"),
    Path("internal/ai/agent_handlers.go"),
    Path("internal/ai/handlers.go"),
    Path("internal/spam/handlers.go"),
    Path("internal/control/handlers.go"),
    Path("internal/tags/handlers.go"),
    Path("internal/admin/granular_bans.go"),
    Path("internal/admin/literals.go"),
    Path("internal/admin/moderation.go"),
    Path("internal/admin/moderation_alerts.go"),
    Path("internal/comparison/jce_handlers.go"),
]

FUNC_START = re.compile(r"^func (?:\([^\)]+\) )?\w+\(" )


def find_function_end(lines: list[str], start: int) -> int:
    for i in range(start + 1, len(lines)):
        if FUNC_START.match(lines[i]):
            return i
    return len(lines)


def var_used_after(lines: list[str], current: int, end: int, var: str) -> bool:
    return any(re.search(rf"\b{re.escape(var)}\b", line) for line in lines[current+1:end])


def ok_declared_before(lines: list[str], start: int, current: int) -> bool:
    for line in lines[start:current]:
        if re.search(r"\bok\b\s*:=", line):
            return True
    return False


def fix_file(path: Path):
    text = path.read_text()
    lines = text.splitlines(keepends=True)
    out = list(lines)
    changed = False

    i = 0
    while i < len(out):
        if FUNC_START.match(out[i]):
            func_start = i
            func_end = find_function_end(out, i)
            j = i + 1
            while j < func_end:
                line = out[j]
                new_line = None

                # tenantID, ok := h.requireTenant(w, r) or requireTenant(w, r)
                m = re.match(r"^(\s*)tenantID,\s*ok\s*:=\s*((?:h\.)?requireTenant\(w,\s*r\))$", line)
                if m:
                    indent, rhs = m.group(1), m.group(2)
                    if not var_used_after(out, j, func_end, "tenantID"):
                        if ok_declared_before(out, func_start, j):
                            new_line = f"{indent}_, ok = {rhs}\n"
                        else:
                            new_line = f"{indent}_, ok := {rhs}\n"

                # tenantID := tenant.FromContext(r.Context())
                m = re.match(r"^(\s*)tenantID\s*:=\s*(tenant\.(?:FromContext|IDFromContext)\([^\)]*\))$", line)
                if m and not new_line:
                    indent, rhs = m.group(1), m.group(2)
                    if not var_used_after(out, j, func_end, "tenantID"):
                        new_line = f"{indent}_ = {rhs}\n"
                    else:
                        new_line = f"{indent}tenantID := {rhs}\n"

                # tenantPart := ...
                m = re.match(r"^(\s*)tenantPart\s*[:=]\s*(.+)$", line)
                if m and not new_line:
                    indent, rhs = m.group(1), m.group(2)
                    if not var_used_after(out, j, func_end, "tenantPart"):
                        new_line = f"{indent}_ = {rhs}\n"
                    else:
                        new_line = f"{indent}tenantPart := {rhs}\n"

                # org, ok := ... (organizations)
                m = re.match(r"^(\s*)org,\s*ok\s*:=\s*(.+)$", line)
                if m and not new_line:
                    indent, rhs = m.group(1), m.group(2)
                    if not var_used_after(out, j, func_end, "org"):
                        new_line = f"{indent}_, ok = {rhs}\n"
                    else:
                        new_line = f"{indent}org, ok := {rhs}\n"

                # org := ...
                m = re.match(r"^(\s*)org\s*:=\s*(.+)$", line)
                if m and not new_line:
                    indent, rhs = m.group(1), m.group(2)
                    if not var_used_after(out, j, func_end, "org"):
                        new_line = f"{indent}_ = {rhs}\n"
                    else:
                        new_line = f"{indent}org := {rhs}\n"

                if new_line:
                    out[j] = new_line
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

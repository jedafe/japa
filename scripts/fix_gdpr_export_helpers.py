#!/usr/bin/env python3
"""
Bulk-fix gdpr/store.go and gdpr/service.go:
- Drop tenantID from gdprExportUser* helper signatures.
- Convert exportJSONSlice(..., gen.XParams{UserID: userID}) to exportJSONSlice(..., userID).
- Remove tenantID from all s.store.gdprExportUser*(ctx, userID, tenantID) calls.
"""
from pathlib import Path
import re

STORE = Path("internal/gdpr/store.go")
SERVICE = Path("internal/gdpr/service.go")


def fix_store(text: str) -> str:
    # Remove tenantID from helper signatures that take (userID, tenantID)
    text = re.sub(
        r"(func \(s \*Store\) gdpr\w+)\(ctx context\.Context, userID, tenantID pgtype\.UUID\)",
        r"\1(ctx context.Context, userID pgtype.UUID)",
        text,
    )
    # Convert exportJSONSlice / exportJSONSliceNoTenant with composite params to positional userID
    text = re.sub(
        r"(exportJSONSlice(?:NoTenant)?)\(ctx, s\.read\.(Gdpr\w+), gen\.\w+Params\{\s*UserID:\s*userID,\s*\}\)",
        r"\1(ctx, s.read.\2, userID)",
        text,
        flags=re.S,
    )
    return text


def fix_service(text: str) -> str:
    # Remove tenantID from all s.store.gdpr* store calls
    text = re.sub(
        r"(s\.store\.gdpr\w+)\(ctx, userID, tenantID\)",
        r"\1(ctx, userID)",
        text,
    )
    return text


if __name__ == "__main__":
    for path, fn in [(STORE, fix_store), (SERVICE, fix_service)]:
        original = path.read_text()
        new_text = fn(original)
        if new_text != original:
            path.write_text(new_text)
            print(f"patched {path}")

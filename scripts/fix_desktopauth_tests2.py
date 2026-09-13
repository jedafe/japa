#!/usr/bin/env python3
"""Add a valid tenant ID to desktopauth test APIKeyContexts that mint tokens."""
from pathlib import Path
import re

p = Path("internal/desktopauth/handlers_test.go")
text = p.read_text()

# For every APIKeyContext literal that is used in a successful ExchangeKey test,
# add TenantID if it has Scopes including api.write.content or [*].
# We do this by adding a TenantID field after KeyID/UserID.

def add_tenant(match):
    body = match.group(1)
    if "TenantID:" in body:
        return match.group(0)
    # Insert TenantID after UserID line
    body = re.sub(
        r"(UserID:\s*uid,\n)",
        r"\1\t\tTenantID: pgtype.UUID{Valid: true},\n",
        body,
    )
    return match.group(0).replace(match.group(1), body)

text = re.sub(
    r"ctx := &publicapi\.APIKeyContext\{(.*?)\}",
    add_tenant,
    text,
    flags=re.S,
)

p.write_text(text)
print("patched desktopauth/handlers_test.go")

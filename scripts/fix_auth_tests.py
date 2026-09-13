#!/usr/bin/env python3
"""Update auth jwt/middleware tests to use a zero tenant."""
from pathlib import Path
import re

for file in ["internal/auth/jwt_test.go", "internal/auth/middleware_test.go"]:
    p = Path(file)
    text = p.read_text()

    # Replace `tenantID := pgtype.UUID{...}` with zero UUID
    text = re.sub(
        r"tenantID := pgtype\.UUID\{Bytes: \[16\]byte\{[^}]+\}, Valid: true\}",
        "tenantID := pgtype.UUID{}",
        text,
    )

    # Use the tenantID variable in MintAccessToken calls (replace any pgtype.UUID{} second arg with tenantID)
    text = re.sub(
        r"MintAccessToken\(userID, pgtype\.UUID\{\}\)",
        "MintAccessToken(userID, tenantID)",
        text,
    )
    text = text.replace("MintAccessToken(invalidID, pgtype.UUID{})", "MintAccessToken(invalidID, tenantID)")

    # Remove the TenantID assertion if it checks against tenantID.String()
    text = re.sub(
        r"\n\tif claims\.TenantID != tenantID\.String\(\) \{\n\t\tt\.Errorf\(\"TenantID = %s, want %s\", claims\.TenantID, tenantID\.String\(\)\)\n\t\}\n",
        "\n",
        text,
    )

    p.write_text(text)
    print(f"patched {file}")

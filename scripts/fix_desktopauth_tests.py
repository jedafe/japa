#!/usr/bin/env python3
"""Remove tenant from desktopauth handler tests."""
from pathlib import Path
import re

p = Path("internal/desktopauth/handlers_test.go")
text = p.read_text()

# Remove `tid := mustUUID(t, ...)` declarations (unused)
text = re.sub(r"\n\ttid := mustUUID\(t, \"[^\"]+\"\)", "", text)

# Remove `created.TenantID != tid` block
text = re.sub(
    r"\n\tif created\.TenantID != tid \{\n\t\tt\.Fatal\(\"refresh token tenant ID mismatch\"\)\n\t\}\n",
    "\n",
    text,
)

p.write_text(text)
print("patched desktopauth/handlers_test.go")

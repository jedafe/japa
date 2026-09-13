#!/usr/bin/env python3
"""Remove the obsolete tenant_id column from the fake GetComparisonBySlugOrID row values."""
from pathlib import Path
import re

p = Path("internal/jce/scoring/engine_test.go")
text = p.read_text()

# Remove tenant_id value and update comments
old = """\treturn []any{
\t\tcontentID,                // id
\t\ttenantID,                 // tenant_id
\t\tauthorID,                 // author_id"""
new = """\treturn []any{
\t\tcontentID,                // id
\t\tauthorID,                 // author_id"""
text = text.replace(old, new)

# Update function signature to drop unused tenantID parameter
old = "func getComparisonBySlugOrIDRowValues(tenantID, contentID, authorID pgtype.UUID) []any {"
new = "func getComparisonBySlugOrIDRowValues(contentID, authorID pgtype.UUID) []any {"
text = text.replace(old, new)

# Update call sites
text = re.sub(
    r"\{values: getComparisonBySlugOrIDRowValues\(tenantID, contentID, authorID\)\}",
    "{values: getComparisonBySlugOrIDRowValues(contentID, authorID)}",
    text,
)

p.write_text(text)
print("patched engine_test.go")

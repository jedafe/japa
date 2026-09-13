#!/usr/bin/env python3
"""Add tenant_id to learning track/lesson/module query column lists so generated
row types retain a TenantID field and existing Go conversions continue to compile."""
from pathlib import Path

SQL = Path("internal/db/queries/learning.sql")
text = SQL.read_text()

# Track queries: SELECT id, title, ... -> SELECT id, tenant_id, title, ...
text = text.replace(
    "SELECT id, title, slug, description, difficulty, cover_media_id,",
    "SELECT id, tenant_id, title, slug, description, difficulty, cover_media_id,"
)

# Track detail: lt.id, lt.title, ... -> lt.id, lt.tenant_id, lt.title, ...
text = text.replace(
    "SELECT lt.id, lt.title, lt.slug, lt.description, lt.difficulty,",
    "SELECT lt.id, lt.tenant_id, lt.title, lt.slug, lt.description, lt.difficulty,"
)

# Create/Update track RETURNING lists: id, title -> id, tenant_id, title
text = text.replace(
    "RETURNING id, title, slug, description, difficulty,",
    "RETURNING id, tenant_id, title, slug, description, difficulty,"
)

# Lesson queries: SELECT id, module_id, ... -> SELECT id, tenant_id, module_id, ...
text = text.replace(
    "SELECT id, module_id, title, slug, lesson_type, content, media_id,",
    "SELECT id, tenant_id, module_id, title, slug, lesson_type, content, media_id,"
)

# Lesson detail: ll.id, ll.module_id, ... -> ll.id, ll.tenant_id, ll.module_id, ...
text = text.replace(
    "SELECT ll.id, ll.module_id, ll.title, ll.slug, ll.lesson_type,",
    "SELECT ll.id, ll.tenant_id, ll.module_id, ll.title, ll.slug, ll.lesson_type,"
)

# Lesson create/update RETURNING lists: id, module_id -> id, tenant_id, module_id
text = text.replace(
    "RETURNING id, module_id, title, slug, lesson_type, content,",
    "RETURNING id, tenant_id, module_id, title, slug, lesson_type, content,"
)

# Module queries: SELECT id, track_id, ... -> SELECT id, tenant_id, track_id, ...
text = text.replace(
    "SELECT id, track_id, title, slug, description,",
    "SELECT id, tenant_id, track_id, title, slug, description,"
)

# Module create/update RETURNING: id, tenant_id, track_id... already has tenant_id in some?
# Make sure id, track_id becomes id, tenant_id, track_id
if "RETURNING id, track_id, title, slug, description, sort_order," in text:
    text = text.replace(
        "RETURNING id, track_id, title, slug, description, sort_order,",
        "RETURNING id, tenant_id, track_id, title, slug, description, sort_order,"
    )

SQL.write_text(text)
print("patched learning.sql")

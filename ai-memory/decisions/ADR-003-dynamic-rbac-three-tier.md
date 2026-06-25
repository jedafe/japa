# ADR-003: Dynamic Three-Tier RBAC Over Hardcoded Roles

**Status:** Accepted

**Context:** Early design treated some role assignments as fixed in code (e.g. "which roles can use Page Builder"). This was explicitly corrected mid-planning.

**Decision:** User Groups → User Roles → Permissions, fully admin-configurable, with only three system defaults (`super_admin`, `members`, `guest`) that aren't admin-deletable.

**Rationale:** Hardcoding role checks means every new use case requiring different access (Page Builder, language pack management, blog editorial permissions) requires a code change. A dynamic permission-node system lets admins compose access without touching code.

**Consequences:** Every new feature with access control needs to define its permission node(s) up front rather than reach for a fixed role name. This is now the default pattern applied throughout v1 (Page Builder's `content.page_builder.create`, language pack managers, blog's granular `blog.*` nodes).

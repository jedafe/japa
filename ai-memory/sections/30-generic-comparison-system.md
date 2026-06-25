# Section 30 — Generic Comparison System

**Status:** ✅ Locked (restored post-audit — see ADR-029)

## Decided

Two layers, not one — a quick embedded block for casual comparisons, and a full standalone content type for structured ones. Both were already referenced from Sections 15, 16, 25, and 26 as "the Generic Comparison System," but had no canonical definition until now; this section is that definition.

**Layer 1 — Comparison block** (TipTap, Section 15 / Page Builder "Comparison Embed," Section 16): author defines rows (dimensions) and columns (items) inline inside any post, fills in values, renders as a responsive table — horizontal scroll with sticky first column on mobile. This is also why the TipTap Table extension was deliberately excluded (Section 15) — this block covers the structured-tabular need instead of a generic table.

**Layer 2 — Comparison content type:**

```sql
comparison_items (
  id, content_id FK,            -- content_type = 'comparison' in Section 14
  title, description, image_id FK NULL,
  geo_space_id FK NULL,         -- optional link to a Geo Space
  position INT, created_at
)
comparison_dimensions (
  id, content_id FK, label TEXT,
  dimension_type TEXT NOT NULL, -- text | rating | boolean | number | currency
  position INT
)
comparison_values (
  dimension_id FK, item_id FK,
  value TEXT, value_rating DECIMAL NULL, value_boolean BOOLEAN NULL,
  created_at,
  PRIMARY KEY (dimension_id, item_id)
)
```

Registered as a `comparison` content type under Section 14's discriminator pattern — same storage convention as every other content type, not a parallel system. Routed at `/compare/[entity-type]/[slug-or-id-list]` (Section 23).

AI-generatable via the Content Agent (Section 20): a prompt like "compare studying in Canada vs UK for Nigerians" queries Geo Spaces and existing content, then produces a structured comparison through the standard HITL pipeline before publishing — comparisons touch exactly the immigration/financial decision-making content category where the mandatory human-in-the-loop review applies.

Comparison pages deliberately skip JSON-LD structured data (Section 26) — no clean schema.org fit for this shape of content. Comparison Matrix is a shared frontend component (Section 23 / `ai-memory/conventions/svelte-component-patterns.md`) distinct from Entity Card, since it needs field-by-field, entity-by-column layout rather than a side-by-side card arrangement. Mentorship Matching (V2) explicitly reuses this page pattern for its own comparison-style matching UI rather than building a separate one.

## See Also

ADR-029. Section 14 (content type registration), Section 15 (TipTap block, Table extension exclusion), Section 16 (Page Builder embed), Section 20 (AI generation, HITL), Section 23 (route, Comparison Matrix component), Section 25 (comparison sponsored ad row), Section 26 (structured data exclusion).

## Admin CRUD: Comparison Types

Admin can define named Comparison Types with their own custom field definitions — a "Flight Booking Comparison" type has different fields from a "University Comparison" type. This uses the Section 06 Custom Field Group system.

```sql
-- comparison_type is a space_type-style registry of named comparison schemas
CREATE TABLE comparison_type_definitions (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       uuid NOT NULL REFERENCES tenants(id),
  name            text NOT NULL,     -- 'Flight Booking', 'Universities', 'Banks', 'Visa Types'
  slug            text NOT NULL,
  description     text,
  icon            text,
  field_group_id  uuid NOT NULL REFERENCES custom_field_groups(id),
  -- Fields in this group become the comparison dimensions for items of this type
  -- Each field = one comparison_dimension row when a comparison is created
  item_label      text NOT NULL DEFAULT 'Item',   -- 'University', 'Flight', 'Bank'
  is_system       boolean NOT NULL DEFAULT false,  -- seeded defaults (Universities)
  created_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE(tenant_id, slug)
);

-- Access control: which UserGroups (and AI agents) can fill this comparison type
CREATE TABLE comparison_type_access (
  type_id         uuid NOT NULL REFERENCES comparison_type_definitions(id) ON DELETE CASCADE,
  user_group_id   uuid REFERENCES user_groups(id),  -- NULL = AI agent only
  can_create      boolean NOT NULL DEFAULT false,
  can_fill        boolean NOT NULL DEFAULT true,     -- fill dimensions for existing comparisons
  ai_can_fill     boolean NOT NULL DEFAULT false,    -- allow AI agent to auto-fill
  PRIMARY KEY (type_id, COALESCE(user_group_id, '00000000-0000-0000-0000-000000000000'))
);
```

## Admin Panel Interface

```
Admin → Content → Comparison Types
  List: name, slug, field count, comparison count, enabled/disabled
  Create/Edit:
    - Name, slug, icon, item_label
    - Field Group: select existing or create new (opens Custom Field Builder inline)
    - Access control: which UserGroups can create / fill
    - AI fill: toggle AI agent auto-fill permission
  Delete: only if no comparisons exist using this type
```

## Create Flow (for eligible UserGroup or AI)

When a user creates a new Comparison (content type = `comparison`):
1. Select Comparison Type (from types the user's group has `can_create` access to)
2. Name the comparison (e.g. "Best Universities in Canada for Nigerian Students")
3. Add items (each item = a `comparison_item` row with a title and optional linked entity)
4. Fill dimensions: the comparison type's custom field group fields become dimension rows
   - Eligible users fill manually
   - AI auto-fill: if `ai_can_fill=true`, AI agent pre-fills values via HITL pipeline
5. Publish (standard content publish flow)

## Pre-seeded Comparison Types (system=true)

- `universities` — linked to `university_fields` group, `ai_can_fill=true`
- `visa_types` — for immigration comparison content
- `banks` — for banking/financial comparison content
- `courses` — for learning track comparison content

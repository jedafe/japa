# ADR-030: Universal Custom Field Architecture

**Status:** Accepted

**Context:** The platform has multiple entity types (Space Types, Content Types, User Profiles, Organization Profiles, Forms, Tags, Tickets) each needing extensible structured fields. The original Section 06 stub described a JSONB-based Custom Field Builder and Form Builder as separate concerns without a unified schema. As the platform grew in scope, it became clear that maintaining separate field systems per entity would create duplication, inconsistent RBAC patterns, and maintenance burden.

**Decision:** One universal field definition system underlies everything. `custom_field_groups` hold named collections of `custom_fields`. Groups are assigned to entity classes via `custom_field_group_assignments` (the join layer). Per-field RBAC is handled by `custom_field_permissions (field_id, user_group_id, can_view, can_edit)`. A `platform_embed` field type handles URL-to-embed resolution for YouTube, GitHub, Twitter/X, Vimeo, Spotify, Figma, CodePen, and Google Maps — avoiding separate one-off embed implementations per field context. The Form Builder reuses the same `custom_fields` infrastructure with `forms`, `form_steps`, `form_submissions`, and `form_submission_triggers` (workflow automation on submit). Default groups are seeded at install and marked `is_system=true`.

**Rationale:** A single field system means one Admin UI to learn, one RBAC pattern to enforce, one Meilisearch indexing strategy to maintain, and no risk of diverging field behaviors across entity types. The platform_embed type eliminates five separate YouTube/GitHub/etc. one-off implementations that would otherwise accumulate over time.

**Consequences:** Every entity type that wants custom fields (Space Types, Content Types, User Profiles, Organizations, Forms, Tags, Tickets) must attach a `custom_field_group` rather than adding columns to their own tables. Field values are stored in `custom_field_values` (queryable) and denormalized into the entity's `field_values JSONB` for fast reads. Schema migrations for new field types require adding to the `field_type` enum and handling in the embed resolver.

# ADR-045: Dedicated Post Create Page (/create)

**Status:** Accepted

**Context:** Content creation was previously via inline editors embedded in context (e.g. a "create post" form at the top of a Space feed). This approach doesn't scale to complex content types with many custom fields, block types, and sidebar settings.

**Decision:** Dedicated full-page editor at `/create` with a type selector → `/create/[content_type]` → full Notion-style TipTap editor. Right sidebar hosts: tags, featured image (Media Library modal), Space assignment, scheduled publish, custom fields for the content type, visibility, SEO settings, and canonical URL. Block availability filtered per UserGroup via `editor_block_permissions (block_type, user_group_id, enabled)`. Autosave every 30 seconds via PATCH to draft revision. Post edit lock via Valkey key `jc:editor_lock:{content_type}:{content_id}` (TTL 120s) prevents concurrent edit conflicts.

**Rationale:** A dedicated creation surface allows full-screen focus, sidebar settings without cluttering the editor, and consistent UX regardless of content type. Unified `/create` entry point also simplifies navigation ("where do I write?") and makes the block-access-control surface manageable in one Admin settings page.

**Consequences:** The `/create` route must be feature-flag-aware — if a content type's feature is disabled, its type card is hidden from the selector. The edit lock Valkey key must be refreshed on each autosave to extend the TTL as long as the user is actively editing.

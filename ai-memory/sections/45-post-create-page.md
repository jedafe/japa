# Section 45 — Dedicated Post Create Page (/create)

**Status:** ✅ Locked

## Concept

A dedicated creation surface at `japaconnect.com/create` — a Notion-inspired full-page editor where users can create any content type they have access to. All TipTap blocks and third-party/custom blocks are available (admin-configurable per UserGroup per block). This replaces the embedded inline editors and provides a focused, distraction-free authoring experience.

## Route Structure

```
/create                     → Type selector (shows only types the user can create)
/create/[content_type]      → Full editor for that content type
/create/[content_type]/[id] → Edit existing post (same editor, pre-populated)
```

## Type Selector

```svelte
<!-- Shows content types gated by RBAC permission node: content.{type}.create -->
<!-- E.g. blog_post requires blog.create, discussion requires forum.create -->
<ContentTypeSelector>
  {#each availableTypes as type}
    <TypeCard icon={type.icon} label={type.label} description={type.description}
              href="/create/{type.slug}" />
  {/each}
</ContentTypeSelector>
```

## Editor Layout

```
Header bar:    [Back] [Content Type badge] [Space selector] [Status: Draft] [Publish ▾]
               [Save as Draft] [Preview] [Settings ⚙]

Main area:     Full-width TipTap editor, Notion-style (click-to-type anywhere)
               Slash commands (/) open block picker
               All blocks available to this UserGroup shown in slash menu

Right sidebar: (collapsible)
  - Tags / Categories
  - Featured image
  - Space / Geo Space assignment
  - Scheduled publish date
  - Custom fields for this content type (from Section 06 field groups)
  - Visibility (public / members / private)
  - SEO settings (meta title, meta description, canonical URL)
  - Related posts (manual pin)
```

## Block Access Control Per UserGroup

Admin configures which UserGroup has access to which blocks in Admin Panel → Settings → Content → Editor Blocks:

```sql
CREATE TABLE editor_block_permissions (
  block_type      text NOT NULL,     -- 'comparison_embed'|'video_embed'|'code_block'|'ai_generate'|etc.
  user_group_id   uuid NOT NULL REFERENCES user_groups(id) ON DELETE CASCADE,
  enabled         boolean NOT NULL DEFAULT true,
  PRIMARY KEY (block_type, user_group_id)
);
```

```go
// Slash command filter — only shows blocks the current user's group can access
func GetAvailableBlocks(ctx context.Context, userID uuid.UUID) ([]BlockDef, error) {
  groups := getUserGroups(ctx, userID)
  return queries.GetEnabledBlocksForGroups(ctx, extractGroupIDs(groups))
}
```

## V1 Block Catalog (Full)

**Text / Structure**
- Paragraph, Heading (H1-H6), Blockquote, Code Block (syntax highlight), Divider, Callout, Details/Summary (accordion)

**Media**
- Image (from Media Library modal or URL), Video (MP4/embed), YouTube/Vimeo/Spotify embed, GIF, File attachment

**Layout**
- Columns (2 or 3), Table of Contents (auto-generated from headings), Columns Comparison (Layer 1 inline comparison)

**Interactive**
- FAQ Accordion, Steps/Process (numbered), Countdown Widget, Stats Counter, Poll (forum threads only)

**Content**
- Comparison Embed (link to a comparison_type record), Related Posts, Newsletter Subscribe CTA, Social Share buttons, Feedback button

**Advanced (admin/verified UserGroups)**
- AI Generate block (calls content_agent, result editable), Embed (iFrame, SSRF-protected)

## Custom Fields in Editor

Custom fields for the selected content type (from Section 06) appear in the right sidebar, not the editor body — this keeps the main writing surface clean while making structured metadata available without leaving the page.

## Autosave

Every 30 seconds, the editor posts a PATCH to save a draft revision. A Valkey key `jc:editor_lock:{content_type}:{content_id}` with TTL 120s prevents concurrent edits — if two sessions try to edit the same post, the second receives a "Someone else is editing this" warning.

## See Also

Section 14 (content types), Section 15 (TipTap extensions), Section 16 (Page Builder for pages — distinct from this editor), Section 34 (Media Library modal). ADR-045.

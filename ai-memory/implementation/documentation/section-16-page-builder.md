# Section 16 — Page Builder: Implementation Complete

**Date:** 2026-06-25
**Phase:** 2
**Section:** 16
**Status:** Complete

## Overview

Section 16 implements the drag-and-drop page builder that lets users compose
content pages from reusable blocks. The backend provides a block template
registry with 11 block types and a single HTTP endpoint that returns the
registry to the editor. The frontend ships 11 block components, a
PageBuilder wrapper with `@dnd-kit/svelte` drag-and-drop reordering, a block
picker dialog with search and category grouping, a settings panel that
dynamically generates form fields from block props, and a toolbar with
add/preview/save controls. Page layouts are stored in the
`contents.field_values` JSONB column introduced in Section 14, using a
versioned schema.

## Components Implemented

### Backend: Block Template Registry (2 files)

**block_templates.go** (209 lines)

- `BlockTemplate` struct with `Type`, `Name`, `Description`, `Icon`,
  `DefaultProps`, and `Category` fields (JSON-serializable)
- `BlockTemplateRegistry` holds all 11 block types, constructed once at
  package init via `newDefaultRegistry()`
- Five block categories: `layout`, `content`, `media`, `interactive`, `data`
- `GetBlockTemplates()` returns a defensive copy of all templates — callers
  cannot mutate the registry through the returned slice
- `GetBlockTemplateByType(blockType)` returns a single template by type string
  with a boolean found indicator
- `BlockTypeCount()` returns the number of registered types
- Each block type carries sensible default props so the editor can instantiate
  a new block without a server round-trip

**block_handlers.go** (22 lines)

- `ListBlockTypes` handler on the existing `Handlers` struct
- `GET /api/v1/page-builder/block-types` — requires authentication (via
  `auth.RequireAuth`) but no special permission, because it returns a static
  registry, not tenant data
- Returns JSON `{"data": {"block_types": [...]}}` in the standard envelope
- The front-end calls this when the page builder editor mounts to populate the
  block picker

### Backend: Tests (1 file)

**block_templates_test.go** (147 lines)

- 9 unit tests covering the registry and handler:
  - `TestBlockTemplateCount` — verifies exactly 11 block types registered
  - `TestGetBlockTemplatesReturnsAll` — all 11 returned by the getter
  - `TestGetBlockTemplatesReturnsCopy` — mutating the returned slice does not
    affect subsequent calls (defensive copy verification)
  - `TestBlockTemplatesRequiredFields` — every template has non-empty Type,
    Name, Category, Icon, Description, and a valid category value
  - `TestBlockTemplatesDefaultPropsNotNil` — every template has non-nil
    DefaultProps
  - `TestGetBlockTemplateByType` — all 11 types found by lookup, unknown type
    returns false
  - `TestBlockTemplatesJSONSerializable` — round-trip JSON marshal/unmarshal
    preserves all 11 templates
  - `TestListBlockTypesHandler` — handler returns 200, correct content type,
    and 11 block types in the response envelope

### Server Wiring

- `contentHandlers.ListBlockTypes` registered at `GET /page-builder/block-types`
  under the auth middleware group
- 3 new permission keys bootstrapped at startup (added to the existing
  `bootstrapPermissions` slice alongside the 33 from Sections 05, 06, 14,
  and 15):

| Key | Description |
|-----|-------------|
| `content.page_builder.create` | Create pages with page builder |
| `content.page_builder.edit` | Edit pages with page builder |
| `content.page_builder.delete` | Delete page builder pages |

All 36 permissions (9 Section 05 + 11 Section 06 + 11 Section 14 + 2 Section 15
+ 3 Section 16) are assigned to the `super_admin_role`, which is granted to the
`super_admin` system group. Every operation is idempotent.

### Frontend: Type Definitions (1 file)

**types.ts** (33 lines)

- `BlockDefinition` — mirrors the backend `BlockTemplate` shape (type, name,
  description, icon, default_props, category)
- `BlockInstance` — a single block placed on a page (block_id, block_type,
  order, props)
- `PageSchema` — the full page schema stored in `contents.field_values`
  (schema_version, blocks array)
- `BlockCategory` — union type of the five valid category strings

### Frontend: Client-Side Registry (1 file)

**registry.ts** (179 lines)

- Mirrors the backend `BlockTemplateRegistry` with all 11 block types and
  their default props — the backend is the source of truth, this is a local
  cache for instant block creation in the editor UI
- `getBlockTypes()` — returns a copy of all block definitions
- `getBlockTypeByType(type)` — lookup by type string
- `getBlockTypesByCategory(category)` — filter by category
- `createBlockInstance(type)` — creates a new `BlockInstance` with a
  `crypto.randomUUID()` block_id and `structuredClone`'d default props;
  throws if the block type is not registered

### Frontend: PageBuilder Wrapper (1 file)

**PageBuilder.svelte** (148 lines)

- Main wrapper component accepting `blocks` (BlockInstance[]), optional
  `readOnly` (boolean), and `onChange` callback
- Uses Svelte 5 runes (`$state`, `$derived`, `$props`)
- Drag-and-drop via `@dnd-kit/svelte` — `<DragDropProvider>` wraps the block
  list, providing the drag-and-drop context for all child sortables
- `onDragEnd` handler uses `isSortableOperation()` type guard to extract
  `source.initialIndex` and `target.index` from the drag operation, then
  splices the blocks array to reorder
- Block component map: `Record<string, Component>` mapping block_type to the
  corresponding Svelte component
- `deleteBlock`, `duplicateBlock`, `addBlock` CRUD operations
- `emitChange` re-indexes the `order` field on every change to keep it
  consistent
- Empty state with dashed border placeholder when no blocks are present
- Unknown block types render a red error placeholder instead of crashing

### Frontend: SortableBlock Wrapper (1 file)

**SortableBlock.svelte** (107 lines)

- Per-block wrapper providing the edit-mode chrome: drag handle (GripVertical
  icon), block type label, duplicate button (Copy icon), delete button
  (Trash2 icon)
- Accepts `id`, `blockType`, `blockLabel`, `index`, `readOnly`, `children`
  (Svelte 5 snippet), `onDelete`, `onDuplicate` props
- Uses `createSortable({ get id(), get index(), get disabled() })` from
  `@dnd-kit/svelte/sortable` — getters ensure reactive prop propagation
  into the drag-and-drop system
- `{@attach sortable.attach}` on the wrapper div connects the element as
  the sortable's draggable/droppable target
- `{@attach sortable.attachHandle}` on the grip handle button restricts
  drag initiation to the handle, not the entire block
- `sortable.isDragging` drives an `opacity-40` class on the wrapper during
  drag for visual feedback
- Read-only mode passes `disabled: true` to `createSortable`, hiding the
  edit toolbar and preventing drag interactions
- Uses Lucide icons for all action buttons

### Frontend: BlockPicker Dialog (1 file)

**BlockPicker.svelte** (195 lines)

- Modal dialog for adding new blocks — fixed overlay with backdrop blur
- Search input filtering by name, description, and type
- Blocks grouped by category in a fixed order: layout, content, media,
  interactive, data
- Each block type shown with its Lucide icon, name, and description
- Icon map translating backend icon names to Lucide components (11 icons)
- Keyboard support: Escape closes the dialog
- Backdrop click closes the dialog
- Empty state when search yields no matches
- Category headers with uppercase labels

### Frontend: BlockSettingsPanel (1 file)

**BlockSettingsPanel.svelte** (296 lines)

- Side panel for editing a selected block's properties
- Dynamic form generation from block definition's `default_props` — infers
  field type from the prop's default value:
  - `boolean` default renders a toggle switch (role="switch", aria-checked)
  - `number` default renders a number input
  - `object`/`array` default renders a text input (JSON editing)
  - Known keys (`content`, `quote`) render as textarea
  - URL-suffixed keys (`_url`, `_image`, `background_image`) render as URL
    input with `https://` placeholder
  - Keys with entries in `SELECT_OPTIONS` render as dropdown selects
  - Everything else renders as a text input
- `SELECT_OPTIONS` map provides predefined options for layout, style, variant,
  display_mode, sort_order, sort_by fields
- Draft state via `$state` — changes are not applied until the user clicks
  "Apply"
- `structuredClone` for isolating draft props from the live block
- Header shows block icon, name, and description
- Footer with Cancel and Apply actions
- Humanizes snake_case prop keys into Title Case labels

### Frontend: PageBuilderToolbar (1 file)

**PageBuilderToolbar.svelte** (82 lines)

- Top toolbar with four controls: Add Block button (primary), block count
  display with Layers icon, Preview/Edit toggle, Save button
- `blockCount`, `isPreview`, `onAddBlock`, `onTogglePreview`, `onSave` props
- Preview toggle button changes style when active (blue highlight)
- `role="toolbar"` with `aria-label` for accessibility
- Responsive layout with flex and spacer div

### Frontend: Block Components (11 files)

All in `apps/main/src/lib/page-builder/blocks/`:

**HeroBlock.svelte** (64 lines)

- Full-width hero section with background image, dark overlay (configurable
  opacity), title (h1), subtitle, and CTA link
- Responsive typography scaling from text-4xl to text-6xl
- Edit-mode ring indicator

**RichTextBlock.svelte** (48 lines)

- Embeds the Section 15 TipTap `Editor` component for inline rich text editing
- Read-only mode renders parsed HTML with `prose` typography classes
- `safeParseHTML` helper detects whether content is TipTap JSON or raw HTML

**ImageBlock.svelte** (120 lines)

- Three layout modes: single image, grid (2-4 columns), carousel
- Carousel has prev/next navigation buttons and dot indicators
- Caption support with `figcaption`
- Empty state placeholder when no images added

**CTABlock.svelte** (49 lines)

- Call-to-action button with three style variants: primary (blue), secondary
  (gray), outline (bordered)
- Renders as `<a>` when URL is provided, `<button>` when only text is set
- Empty state placeholder when both text and URL are missing

**StatsBlock.svelte** (45 lines)

- Statistics grid with responsive columns (2 on mobile, 3 on tablet, 4 on
  desktop)
- Each stat item shows optional icon emoji, large value, and label
- Empty state placeholder when no items added

**FAQBlock.svelte** (83 lines)

- Accordion layout with expand/collapse toggle per question
- `openIndices` state tracked as a `Set<number>` for multi-open support
- Animated chevron rotation on expand
- Optional title heading above the accordion
- Empty state placeholder when no questions added

**TestimonialBlock.svelte** (74 lines)

- Blockquote with decorative quote mark SVG
- Author avatar (image URL or initial letter fallback), name, and role
- Empty state placeholder when no quote or author set

**ComparisonBlock.svelte** (35 lines)

- Placeholder for Section 30 comparison system integration
- Shows comparison_id and a note that rendering is deferred
- Empty state placeholder when no comparison_id set

**FormEmbedBlock.svelte** (42 lines)

- Placeholder for Section 06 form system integration
- Configurable show_title and show_border display options
- Empty state placeholder when no form_id set

**SpacerBlock.svelte** (34 lines)

- Three visual styles: space (empty vertical gap), line (horizontal rule),
  dots (three centered dots)
- Configurable height in pixels via inline style

**DynamicListingBlock.svelte** (41 lines)

- Placeholder for Section 32 content feed integration
- Shows content_type and limit count
- Empty state placeholder when no content_type set

**blocks/index.ts** (11 lines)

- Barrel exports for all 11 block components
- Single import point: `import { HeroBlock, RichTextBlock, ... } from './blocks'`

### Frontend: Module Index (1 file)

**index.ts** (7 lines)

- Barrel exports for the entire page builder module
- Exports registry functions, all wrapper components, and all type definitions

## Storage Format

Page layouts are stored in the `contents.field_values` JSONB column (introduced
in Section 14) using this schema:

```json
{
  "schema_version": 1,
  "blocks": [
    {
      "block_id": "550e8400-e29b-41d4-a716-446655440000",
      "block_type": "hero",
      "order": 0,
      "props": {
        "title": "Welcome to JapaConnect",
        "subtitle": "Your community abroad",
        "background_image": "https://...",
        "cta_text": "Get Started",
        "cta_url": "/signup",
        "overlay_opacity": 0.4
      }
    },
    {
      "block_id": "660e8400-e29b-41d4-a716-446655440001",
      "block_type": "rich_text",
      "order": 1,
      "props": {
        "content": "{\"type\":\"doc\",\"content\":[...]}"
      }
    }
  ]
}
```

- `schema_version` allows future migrations of the block schema without
  breaking existing pages
- `block_id` is a UUID generated client-side via `crypto.randomUUID()`
- `order` is an integer index, re-computed on every change by `emitChange`
- `props` is a free-form object matching the block type's `default_props` shape

## Block Type Catalog

| # | Type | Name | Category | Default Props |
|---|------|------|----------|---------------|
| 1 | `hero` | Hero Section | layout | title, subtitle, background_image, cta_text, cta_url, overlay_opacity |
| 2 | `rich_text` | Rich Text | content | content (TipTap JSON) |
| 3 | `image` | Image / Gallery | media | images (array), caption, layout (single/grid/carousel) |
| 4 | `cta` | Call to Action | interactive | text, url, style (primary/secondary/outline), variant |
| 5 | `stats` | Statistics | data | items (array), layout |
| 6 | `faq` | FAQ Accordion | interactive | questions (array), title |
| 7 | `testimonial` | Testimonial | content | quote, author, role, avatar_url |
| 8 | `comparison` | Comparison | data | comparison_id, display_mode (table/cards/inline) |
| 9 | `form` | Form Embed | interactive | form_id, show_title, show_border |
| 10 | `spacer` | Spacer / Divider | layout | height (px), style (space/line/dots) |
| 11 | `dynamic_listing` | Dynamic Listing | data | content_type, space_id, limit, sort_by, sort_order |

## Test Summary

| Package | Test Count | Result |
|---------|-----------|--------|
| internal/config | 4 | PASS |
| internal/server | 9 | PASS |
| internal/tenant | 3 | PASS |
| internal/auth | 22 | PASS |
| internal/rbac | 4 | PASS |
| internal/features | 5 | PASS |
| internal/geoip | 7 | PASS |
| internal/valkey | 6 | PASS |
| internal/customfields | 37 | PASS |
| internal/spaces | 13 | PASS |
| internal/content | 19 | PASS |
| internal/renderer | 2 | PASS |
| internal/mentions | 5 | PASS |
| internal/linkpreview | 6 | PASS |
| **Total** | **142** | **ALL PASS** |

Note: internal/content went from 10 tests (Section 14) to 19 tests (+9 from
block_templates_test.go).

## Build Verification

| Command | Result |
|---------|--------|
| `go build ./...` | success |
| `go vet ./...` | clean |
| `go test ./... -count=1` | ok — all 14 packages pass |
| Frontend TypeScript check | clean |
| Frontend build | success |

## API Routes Added

1 route registered under `/api/v1`:

| # | Method | Path | Auth | Permission | Description |
|---|--------|------|------|------------|-------------|
| 1 | GET | `/page-builder/block-types` | Required | none (any authenticated user) | Return the block template registry |

## Permission Keys Registered

3 new permission keys bootstrapped at server startup:

| Key | Description |
|-----|-------------|
| `content.page_builder.create` | Create pages with page builder |
| `content.page_builder.edit` | Edit pages with page builder |
| `content.page_builder.delete` | Delete page builder pages |

## Files Created

| # | File | Purpose |
|---|------|---------|
| 1 | `internal/content/block_templates.go` | Block template registry (11 types, 5 categories) |
| 2 | `internal/content/block_handlers.go` | GET /page-builder/block-types handler |
| 3 | `internal/content/block_templates_test.go` | 9 registry and handler tests |
| 4 | `apps/main/src/lib/page-builder/types.ts` | TypeScript type definitions (BlockDefinition, BlockInstance, PageSchema) |
| 5 | `apps/main/src/lib/page-builder/registry.ts` | Client-side block registry mirroring backend |
| 6 | `apps/main/src/lib/page-builder/PageBuilder.svelte` | Main wrapper with @dnd-kit/svelte DragDropProvider |
| 7 | `apps/main/src/lib/page-builder/SortableBlock.svelte` | Per-block sortable wrapper with createSortable |
| 8 | `apps/main/src/lib/page-builder/BlockPicker.svelte` | Modal block picker with search and categories |
| 9 | `apps/main/src/lib/page-builder/BlockSettingsPanel.svelte` | Dynamic property editor panel |
| 10 | `apps/main/src/lib/page-builder/PageBuilderToolbar.svelte` | Top toolbar (add, preview, save) |
| 11 | `apps/main/src/lib/page-builder/index.ts` | Module barrel exports |
| 12 | `apps/main/src/lib/page-builder/blocks/HeroBlock.svelte` | Hero section block |
| 13 | `apps/main/src/lib/page-builder/blocks/RichTextBlock.svelte` | Rich text block (embeds TipTap editor) |
| 14 | `apps/main/src/lib/page-builder/blocks/ImageBlock.svelte` | Image/gallery block (single/grid/carousel) |
| 15 | `apps/main/src/lib/page-builder/blocks/CTABlock.svelte` | Call-to-action button block |
| 16 | `apps/main/src/lib/page-builder/blocks/StatsBlock.svelte` | Statistics grid block |
| 17 | `apps/main/src/lib/page-builder/blocks/FAQBlock.svelte` | FAQ accordion block |
| 18 | `apps/main/src/lib/page-builder/blocks/TestimonialBlock.svelte` | Testimonial quote block |
| 19 | `apps/main/src/lib/page-builder/blocks/ComparisonBlock.svelte` | Comparison embed placeholder |
| 20 | `apps/main/src/lib/page-builder/blocks/FormEmbedBlock.svelte` | Form embed placeholder |
| 21 | `apps/main/src/lib/page-builder/blocks/SpacerBlock.svelte` | Spacer/divider block |
| 22 | `apps/main/src/lib/page-builder/blocks/DynamicListingBlock.svelte` | Dynamic listing placeholder |
| 23 | `apps/main/src/lib/page-builder/blocks/index.ts` | Block component barrel exports |

## Files Modified

| # | File | Change |
|---|------|--------|
| 1 | `internal/server/server.go` | Added `GET /page-builder/block-types` route; added 3 permission keys to `bootstrapPermissions` |

## Key Decisions

- **@dnd-kit/svelte for drag-and-drop** — the implementation uses
  `@dnd-kit/svelte` (v0.5.0), the official Svelte 5 port of the dnd-kit
  library. It provides `DragDropProvider` (context component), `createSortable`
  (primitive for sortable items), and `isSortableOperation` (type guard for
  drag events). The `createSortable` primitive accepts reactive getters for
  `id`, `index`, and `disabled`, and returns `attach` / `attachHandle`
  functions used with Svelte 5's `{@attach}` directive. The `DragDropProvider`
  fires `onDragEnd` with a `DragOperationSnapshot` containing `source` (a
  `SortableDraggable` with `initialIndex`) and `target` (a `SortableDroppable`
  with `index`), which the handler uses to reorder the blocks array. The
  `isSortableOperation` type guard narrows the operation to sortable-specific
  types. This replaces the earlier `svelte-dnd-action` approach.

- **Reactive getters for createSortable** — `createSortable` uses `$effect`
  internally to sync input properties to the sortable instance. Passing plain
  values (e.g., `{ id, index }`) captures only the initial values and triggers
  Svelte's `state_referenced_locally` warning. Using getters (e.g.,
  `{ get id() { return id; }, get index() { return index; } }`) ensures the
  effects re-read current prop values on every reactive update.

- **Block registry pattern (backend + frontend mirror)** — the backend owns the
  canonical block type definitions (type, name, description, icon, default
  props, category). The frontend mirrors this registry locally for instant
  block creation without a server round-trip. The backend endpoint
  `GET /page-builder/block-types` is available for the editor to fetch the
  authoritative registry on mount, but the local mirror enables the editor to
  function even before the API call completes.

- **Dynamic form generation from block props** — `BlockSettingsPanel` infers
  the form field type from each prop's default value (boolean, number, string,
  object) and a set of known field mappings (SELECT_OPTIONS, TEXTAREA_KEYS,
  URL_SUFFIXES). This means adding a new block type to the registry
  automatically makes its props editable in the settings panel without writing
  any form UI.

- **Block component interface: props + readOnly** — every block component
  accepts its block-specific props as individual Svelte 5 `$props()` plus a
  `readOnly` boolean. In edit mode, components show a blue ring indicator and
  may render inline editors (e.g., RichTextBlock embeds the TipTap Editor).
  In read-only mode, components render clean output without edit indicators.

- **Versioned storage schema** — the `schema_version` field in the PageSchema
  JSON enables future migrations of the block format. If the block schema
  changes, a migration can read old versions and transform them to the new
  format. Version 1 is the initial schema.

- **Defensive copy in GetBlockTemplates** — the Go registry returns a copy of
  the internal slice, preventing callers from accidentally mutating the
  registry. This is verified by `TestGetBlockTemplatesReturnsCopy`.

## Known Issues and Deferrals

- **Touch and keyboard optimization** — `@dnd-kit/svelte` provides built-in
  support for keyboard sorting (via `SortableKeyboardPlugin`) and pointer/touch
  sensors (via `PointerSensor` from `@dnd-kit/dom`). These are registered by
  default on the `DragDropManager`. If keyboard or touch UX needs tuning
  (e.g., custom activation constraints), configure sensors via the
  `DragDropProvider`'s `sensors` prop.

- **Undo/redo deferred** — the page builder does not track an operation history.
  Undo/redo support (likely via a command pattern or state snapshots) is
  deferred. Users must rely on content revisions (Section 14) for rollback.

- **Dynamic listing rendering deferred** — `DynamicListingBlock` and
  `ComparisonBlock` render placeholder UI showing the configured content_type
  or comparison_id. Actual data fetching and rendering depends on Section 32
  (Content Feed) and Section 30 (Comparison System) respectively.

- **Form embed rendering deferred** — `FormEmbedBlock` renders a placeholder.
  Actual form rendering depends on the form system from Section 06 being
  extended with a public form rendering endpoint.

- **Block picker uses local registry, not API** — the BlockPicker reads from
  the client-side `registry.ts` mirror rather than fetching from
  `GET /page-builder/block-types`. The API endpoint exists and is wired, but
  the frontend integration to fetch and use it is deferred to Section 45 (Post
  Create Page) which wires the full content creation flow.

- **Image block uses direct URLs** — the ImageBlock accepts image URLs
  directly in its props. Integration with the media system (Section 34) for
  media library selection and image optimization is deferred.

- **Rich text block stores content as string** — the RichTextBlock's `content`
  prop is stored as a string (which may be TipTap JSON or raw HTML). The
  `safeParseHTML` helper handles both formats. Full TipTap JSON validation and
  normalization is deferred.

## Next Steps

Section 45 — Post Create Page. This section depends on:

- Section 16 page builder (the post create page composes the page builder with
  the TipTap editor into a full content creation form)
- Section 15 block editor (the TipTap editor is embedded in the post form)
- Section 14 content types (posts are saved as content rows)
- Section 06 custom fields (post metadata is stored as field_values)
- Section 05 spaces (posts can be assigned to spaces)

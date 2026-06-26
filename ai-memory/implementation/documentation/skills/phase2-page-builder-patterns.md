# Phase 2 Page Builder Patterns

**Date:** 2026-06-25
**Phase:** 2
**Purpose:** Implementation patterns learned during Section 16 (Page Builder), reusable in future sections.

## Pattern: Block Registry (Backend + Frontend Mirror)

The page builder needs a shared understanding of what block types exist, what
props they accept, and what their defaults are. Rather than hardcoding this in
the frontend or making the frontend depend on a server round-trip for every
block creation, the registry is defined in both places:

```go
// Backend: internal/content/block_templates.go
type BlockTemplate struct {
    Type         string         `json:"type"`
    Name         string         `json:"name"`
    Description  string         `json:"description"`
    Icon         string         `json:"icon"`
    DefaultProps map[string]any `json:"default_props"`
    Category     string         `json:"category"`
}

var defaultRegistry = newDefaultRegistry()

func GetBlockTemplates() []BlockTemplate {
    out := make([]BlockTemplate, len(defaultRegistry.templates))
    copy(out, defaultRegistry.templates)
    return out
}
```

```typescript
// Frontend: apps/main/src/lib/page-builder/registry.ts
const BLOCK_TYPES: BlockDefinition[] = [
    {
        type: 'hero',
        name: 'Hero Section',
        description: 'Hero section with title, subtitle, background image, and call-to-action',
        icon: 'layout',
        default_props: { title: '', subtitle: '', /* ... */ },
        category: 'layout',
    },
    // ... 10 more block types
];

export function createBlockInstance(type: string): BlockInstance {
    const definition = getBlockTypeByType(type);
    if (!definition) throw new Error(`Unknown block type: ${type}`);
    return {
        block_id: crypto.randomUUID(),
        block_type: type,
        order: 0,
        props: structuredClone(definition.default_props),
    };
}
```

Key points:

- The backend is the source of truth — the frontend mirror must be kept in
  sync manually (or via the `GET /page-builder/block-types` endpoint)
- `structuredClone` ensures each new block gets an independent copy of the
  default props — mutating one block's props does not affect another
- The defensive `copy()` in Go prevents callers from mutating the registry
  through the returned slice
- Adding a new block type requires updating both the Go registry and the
  TypeScript registry, plus creating the corresponding Svelte component

This pattern is reusable anywhere the backend defines a finite set of
configurable types that the frontend needs to instantiate without a server
call — content types, form field types, notification templates, etc.

## Pattern: Drag-and-Drop with HTML5 API (vs React-Only Libraries)

Most drag-and-drop libraries (`@dnd-kit`, `react-beautiful-dnd`, `SortableJS`)
are framework-specific or add significant bundle size. For vertical list
reordering, the browser's native HTML5 drag-and-drop API is sufficient:

```svelte
<!-- PageBuilder.svelte -->
{#each blocks as block (block.block_id)}
    <div
        draggable={!readOnly}
        ondragstart={(e) => {
            e.dataTransfer.effectAllowed = 'move';
            e.dataTransfer.setData('text/plain', block.block_id);
            dragSourceId = block.block_id;
        }}
        ondragover={(e) => {
            if (readOnly) return;
            e.preventDefault();
            if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
        }}
        ondrop={(e) => handleDrop(e, block.block_id)}
        ondragend={() => { dragSourceId = null; }}
        class:opacity-40={dragSourceId === block.block_id}
        class:scale-95={dragSourceId === block.block_id}
    >
        <!-- block content -->
    </div>
{/each}
```

```typescript
function handleDrop(e: DragEvent, targetId: string) {
    if (readOnly) return;
    e.preventDefault();
    const sourceId = e.dataTransfer?.getData('text/plain');
    if (!sourceId || sourceId === targetId) return;

    const sourceIdx = blocks.findIndex((b) => b.block_id === sourceId);
    const targetIdx = blocks.findIndex((b) => b.block_id === targetId);
    if (sourceIdx === -1 || targetIdx === -1) return;

    const reordered = [...blocks];
    const [moved] = reordered.splice(sourceIdx, 1);
    reordered.splice(targetIdx, 0, moved);
    emitChange(reordered);
    dragSourceId = null;
}
```

Key points:

- `draggable` attribute enables the native drag behavior — no library needed
- `dataTransfer.setData('text/plain', blockId)` carries the dragged item's ID
  through the drag operation
- `e.preventDefault()` in `ondragover` is required — without it, the drop
  event will not fire
- Visual feedback via CSS classes: the dragged item gets `opacity-40` +
  `scale-95`, the drop target gets a blue ring
- `readOnly` mode disables dragging by setting `draggable={false}` and
  returning early from all handlers

This eliminates two npm dependencies (`@dnd-kit/core`, `@dnd-kit/sortable`)
while providing functional drag-and-drop for vertical lists. The trade-off is
limited touch support on mobile — a future iteration can add touch-specific
handlers or switch to a library if mobile reordering becomes a priority.

## Pattern: Dynamic Form Generation from Block Props

The `BlockSettingsPanel` generates a form for editing any block's properties
without knowing the block type in advance. It infers the field type from the
prop's default value and a set of known mappings:

```typescript
type FieldType = 'text' | 'textarea' | 'number' | 'select' | 'boolean' | 'color' | 'url';

function inferFieldType(key: string, defaultValue: unknown): FieldType {
    if (typeof defaultValue === 'boolean') return 'boolean';
    if (typeof defaultValue === 'number') return 'number';
    if (typeof defaultValue === 'object' && defaultValue !== null) return 'text';
    if (TEXTAREA_KEYS.has(key)) return 'textarea';
    if (URL_SUFFIXES.some((suffix) => key.endsWith(suffix))) return 'url';
    if (SELECT_OPTIONS[key]) return 'select';
    return 'text';
}

// Known select options for fields that should render as dropdowns
const SELECT_OPTIONS: Record<string, string[]> = {
    layout: ['single', 'horizontal', 'vertical', 'grid'],
    style: ['primary', 'secondary', 'minimal', 'space', 'divider'],
    variant: ['filled', 'outlined', 'ghost'],
    display_mode: ['table', 'cards', 'inline'],
    sort_order: ['asc', 'desc'],
    sort_by: ['created_at', 'updated_at', 'title', 'views'],
};

// Keys that should always render as textarea
const TEXTAREA_KEYS = new Set(['content', 'quote']);

// Keys that should render as URL input
const URL_SUFFIXES = ['_url', '_image', 'background_image'];
```

The form is then rendered by iterating over the inferred fields:

```svelte
{#each fields as field (field.key)}
    {#if field.type === 'text'}
        <input type="text" bind:value={draftProps[field.key]} />
    {:else if field.type === 'boolean'}
        <button role="switch" aria-checked={!!draftProps[field.key]}
            onclick={() => { draftProps[field.key] = !draftProps[field.key]; }} />
    {:else if field.type === 'select'}
        <select bind:value={draftProps[field.key]}>
            {#each field.options ?? [] as opt}
                <option value={opt}>{humanize(opt)}</option>
            {/each}
        </select>
    {/if}
    <!-- ... number, textarea, url, color -->
{/each}
```

Key points:

- Type inference cascades: check the default value's JS type first, then
  apply key-based heuristics (known textarea keys, URL suffixes, select
  options)
- `humanize()` converts snake_case prop keys to Title Case labels for display
- Draft state is isolated via `structuredClone` — changes are not applied to
  the live block until the user clicks "Apply"
- Adding a new block type with simple props (strings, numbers, booleans,
  enums) requires zero form UI code — the settings panel handles it
  automatically

This pattern is reusable for any admin UI that edits structured JSON
objects — custom field editors, form builders, configuration panels.

## Pattern: Block Component Interface (props + readOnly)

Every block component follows the same interface contract: accept block-specific
props as individual Svelte 5 `$props()` plus a `readOnly` boolean. This
uniformity lets the PageBuilder wrapper render any block type with the same
code:

```svelte
<!-- HeroBlock.svelte -->
<script lang="ts">
    interface Props {
        title?: string;
        subtitle?: string;
        background_image?: string;
        cta_text?: string;
        cta_url?: string;
        overlay_opacity?: number;
        readOnly?: boolean;
    }

    let {
        title = '',
        subtitle = '',
        background_image = '',
        cta_text = '',
        cta_url = '',
        overlay_opacity = 0.4,
        readOnly = false,
    }: Props = $props();

    let wrapperClass = $derived(
        `block-hero relative w-full overflow-hidden${
            !readOnly ? ' ring-2 ring-blue-400/40 hover:ring-blue-400/60' : ''
        }`
    );
</script>
```

The PageBuilder wrapper spreads props onto the component:

```svelte
<!-- PageBuilder.svelte -->
{#if Component}
    <Component {...block.props} {readOnly} />
{:else}
    <div>Unknown block type: {block.block_type}</div>
{/if}
```

Key points:

- All props have defaults — a block with missing props renders gracefully
  (usually an empty state placeholder) rather than crashing
- `readOnly` controls two things: whether edit-mode visual indicators appear
  (blue ring) and whether interactive elements are enabled
- The `{...block.props}` spread means the PageBuilder does not need to know
  what props each block type accepts — it just passes them through
- Unknown block types render a red error placeholder instead of throwing —
  this makes schema mismatches visible in the editor rather than silently
  breaking the page

This pattern is reusable for any component that renders configurable content
blocks — dashboards, widgets, email templates, PDF generators.

## Pattern: Versioned JSONB Storage Schema

Page layouts are stored in the `contents.field_values` JSONB column using a
versioned schema. The `schema_version` field allows future migrations without
breaking existing pages:

```json
{
  "schema_version": 1,
  "blocks": [
    {
      "block_id": "550e8400-e29b-41d4-a716-446655440000",
      "block_type": "hero",
      "order": 0,
      "props": { "title": "...", "subtitle": "..." }
    }
  ]
}
```

Key points:

- `schema_version` is an integer — increment it when the block schema changes
  in a way that requires migration (e.g., renaming a prop, changing the
  structure of a block's props)
- `block_id` is a UUID generated client-side — it is stable across edits and
  used as the Svelte `{#each}` key for efficient DOM updates
- `order` is re-computed on every change by `emitChange()` — this keeps the
  order consistent even after drag-and-drop reordering, without relying on
  array position alone
- `props` is a free-form object — the block type's `default_props` define the
  shape, but the storage does not enforce it. This flexibility allows blocks
  to evolve their props without database migrations

When a schema migration is needed, the read path can check `schema_version`
and transform old formats to the current one:

```typescript
function migratePageSchema(schema: PageSchema): PageSchema {
    if (schema.schema_version === 1) {
        // Apply v1 -> v2 migrations
        schema.blocks = schema.blocks.map(migrateV1Block);
        schema.schema_version = 2;
    }
    return schema;
}
```

This pattern is reusable for any JSONB column that stores structured data
that may evolve over time — custom field values, form submissions,
configuration objects, widget layouts.

## Lesson: HTML5 Drag-and-Drop Requires preventDefault on dragover

The most common mistake with native HTML5 drag-and-drop is forgetting to call
`e.preventDefault()` in the `ondragover` handler. Without it, the browser
treats the drop target as non-droppable and the `ondrop` event never fires.
This is a deliberate design choice in the HTML5 spec — the `dragover` handler
must explicitly opt in to accepting drops.

```javascript
// WRONG — drop event will never fire
ondragover={(e) => {
    if (readOnly) return;
    // missing preventDefault
}}

// RIGHT — drop event fires
ondragover={(e) => {
    if (readOnly) return;
    e.preventDefault();  // required for drop to work
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
}}
```

## Lesson: Defensive Copy for Package-Level Registries

When a Go package exposes a registry via a getter function, always return a
copy of the internal slice. Go slices are references to the same underlying
array — returning the internal slice directly allows callers to mutate the
registry:

```go
// WRONG — caller can mutate the registry
func GetBlockTemplates() []BlockTemplate {
    return defaultRegistry.templates
}

// RIGHT — caller gets a copy, registry is safe
func GetBlockTemplates() []BlockTemplate {
    out := make([]BlockTemplate, len(defaultRegistry.templates))
    copy(out, defaultRegistry.templates)
    return out
}
```

This is verified by `TestGetBlockTemplatesReturnsCopy` — mutate the returned
slice and confirm the next call returns the original values. This pattern
applies to any package-level data structure exposed via a getter — feature
flag registries, permission lists, configuration maps.

## Lesson: structuredClone for Isolating Draft State

When editing a nested object in a form, always clone the source data before
binding it to form inputs. Otherwise, changes to the form immediately mutate
the source object, making "Cancel" impossible:

```typescript
// WRONG — draftProps references the same object as block.props
let draftProps = block.props;

// RIGHT — draftProps is an independent copy
let draftProps = $state(structuredClone(block.props));
```

`structuredClone` is preferred over `JSON.parse(JSON.stringify(...))` because
it handles `undefined`, `Date`, `RegExp`, `Map`, `Set`, and other types that
JSON cannot represent. It is available in all modern browsers and Node.js 17+.

The `$effect` re-clones whenever the selected block changes, ensuring the
draft always starts from the block's current state:

```typescript
$effect(() => {
    if (block) {
        draftProps = structuredClone(block.props);
    } else {
        draftProps = {};
    }
});
```

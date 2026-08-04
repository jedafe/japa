# Modern Geo-Aware Homepage — Plan 3: Admin Landing Page Manager

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the three-pane Landing Page Manager in `apps/admin` — a parameterized editor (by slug) with a section list, live preview, and inspector (Content/Style/Settings tabs). Includes shared sub-editors (GeoText, MediaPicker, IconPicker, BackgroundPicker, CTAEditor, EntityRefPicker, NestedBlocksEditor), the BlockStylePanel, per-block content editors, and the landing pages index DataTable. Reuses the existing Page Builder + media library APIs — no new backend routes.

**Architecture:** The manager lives at `/admin/landing/[slug]` and reads/writes the Page Builder page's `Blocks` JSON via `GET/PATCH /admin/pages/{id}`. The live preview renders the actual `LandingRenderer` from `@japaconnect/landing-blocks` (identical to production). Sub-editors are shared components in the package's `editors/` directory (tree-shaken from the main app build). Media uploads use the existing `MediaPickerModal.svelte` in apps/admin, injected into the package's `MediaPicker` wrapper via dependency injection.

**Tech Stack:** SvelteKit 2, Svelte 5 Runes, Bits UI 2.18, Tailwind CSS v4, TypeScript (zero `any`), lucide-svelte, existing apps/admin DataTable + API client.

## Global Constraints

(Same as Plan 1 — see `docs/superpowers/plans/2026-08-04-modern-geo-aware-homepage-plan1.md` Global Constraints.)

Additional:
- Reuse existing `pagebuilder.blocks.manage` permission — no new RBAC.
- Reuse existing `MediaPickerModal.svelte` from `apps/admin/src/lib/components/ui/` — inject via DI, don't duplicate.
- Reuse existing DataTable spec from `apps/admin/src/lib/components/ui/data-table`.
- Reuse existing admin API client (`apps/admin/src/lib/api/client.ts`) for CSRF + JWT.
- Admin editor components go in `packages/landing-blocks/src/editors/` (tree-shaken from main build via conditional exports).

## Spec Reference

`docs/superpowers/specs/2026-08-04-modern-geo-aware-homepage-design.md` — §7 (Admin Landing Page Manager UI), §8 (shared package editors directory).

## Prerequisites

- Plan 1 complete: `@japaconnect/landing-blocks` with types, blocks, LandingRenderer, geo copy, style system.
- Plan 2 complete: all 23 block primitives + expanded blockRegistry + palette.

---

## File Structure

### Create: `packages/landing-blocks/src/editors/`

| File | Responsibility |
|---|---|
| `index.ts` | Editors barrel export |
| `BlockStylePanel.svelte` | Shared Style tab — Layout/Typography/Color/Border/Spacing/Effects/Transform/Transition/Icon accordions |
| `GeoText.svelte` | Textarea + "Insert placeholder" dropdown + live resolved preview |
| `MediaPicker.svelte` | Wrapper accepting injected `mediaPickerComponent` prop |
| `IconPicker.svelte` | Bits UI Popover + searchable Lucide icon grid |
| `BackgroundPicker.svelte` | Type selector (color/gradient/image/video) + controls |
| `CTAEditor.svelte` | Label, href, variant, icon editor for CTA buttons |
| `EntityRefPicker.svelte` | Search entities by type, return ref |
| `NestedBlocksEditor.svelte` | Mini section list for blocks containing nested blocks |
| `content/HeroContentEditor.svelte` | Hero content tab form |
| `content/CardGridContentEditor.svelte` | CardGrid content tab form |
| `content/EntityCollectionContentEditor.svelte` | EntityCollection content tab form |
| `content/EntityFeatureContentEditor.svelte` | EntityFeature content tab form |
| `content/CarouselContentEditor.svelte` | Carousel content tab form |
| `content/AccordionContentEditor.svelte` | Accordion content tab form |
| `content/MediaGridContentEditor.svelte` | MediaGrid content tab form |
| `content/TabsContentEditor.svelte` | Tabs content tab form |
| `content/VideoContentEditor.svelte` | Video content tab form |
| `content/CountdownContentEditor.svelte` | Countdown content tab form |
| `content/FormEmbedContentEditor.svelte` | FormEmbed content tab form |
| `content/TimelineContentEditor.svelte` | Timeline content tab form |
| `content/MapContentEditor.svelte` | Map content tab form |
| `content/TableContentEditor.svelte` | Table content tab form |
| `content/CtaBannerContentEditor.svelte` | CtaBanner content tab form |
| `content/ComingSoonContentEditor.svelte` | ComingSoon content tab form |
| `content/IconContentEditor.svelte` | Icon content tab form |
| `content/ContentEditor.svelte` | Content (rich text) content tab form |
| `content/QuoteContentEditor.svelte` | Quote content tab form |
| `content/LiveRateTickerContentEditor.svelte` | LiveRateTicker content tab form |
| `content/MediaContentEditor.svelte` | Media content tab form |
| `content/AdvertisementContentEditor.svelte` | Advertisement content tab form |
| `content/SpacerDividerContentEditor.svelte` | SpacerDivider content tab form |
| `content/index.ts` | Maps block type → content editor component |

### Create: `apps/admin/src/routes/landing/`

| File | Responsibility |
|---|---|
| `+page.svelte` | Landing pages index (DataTable) |
| `+page.server.ts` | Fetch list of landing pages |
| `[slug]/+page.svelte` | Three-pane editor |
| `[slug]/+page.server.ts` | Fetch page by slug, palette, themes |

### Modify: `apps/admin/`

| File | Change |
|---|---|
| `src/routes/+layout.svelte` | Add "Landing Pages" nav entry |
| `package.json` | Add `@japaconnect/landing-blocks` dependency |
| `src/app.css` | Add `@source` for `packages/landing-blocks/src` |

---

## Task 1: Admin Package Dependency + Tailwind Source

**Files:**
- Modify: `apps/admin/package.json`
- Modify: `apps/admin/src/app.css`

- [ ] **Step 1: Add dependency to apps/admin/package.json**

Add to `dependencies`:
```json
"@japaconnect/landing-blocks": "workspace:*"
```

- [ ] **Step 2: Add Tailwind content source to apps/admin/src/app.css**

After the `@import 'tailwindcss';` line, add:
```css
@source '../../../packages/landing-blocks/src';
```

- [ ] **Step 3: Run bun install**

Run: `cd /home/jmobile/websites/japa/japaconnect && bun install`
Expected: installs successfully

- [ ] **Step 4: Commit**

```bash
git add apps/admin/package.json apps/admin/src/app.css
git commit -m "feat(admin): add @japaconnect/landing-blocks dependency + Tailwind source"
```

---

## Task 2: GeoText Editor

**Files:**
- Create: `packages/landing-blocks/src/editors/GeoText.svelte`
- Create: `packages/landing-blocks/src/editors/index.ts`

**Interfaces:**
- Consumes: `GeoContext`, `GeoCopyTokens` from `../types`, `resolveString` from `../geo/resolveGeoCopy`
- Produces: `<GeoText value bind:onchange {geoCtx} label />` — textarea + placeholder dropdown + live preview

- [ ] **Step 1: Write the GeoText editor**

Create `packages/landing-blocks/src/editors/GeoText.svelte`:
- Props: `value: string`, `onchange: (v: string) => void`, `geoCtx?: GeoContext`, `label?: string`
- State: `showPreview` = false
- Textarea: `<textarea bind:value class="w-full rounded-[var(--radius)] border border-[var(--color-border)] p-2 text-sm" oninput={() => onchange(value)}>`
- "Insert placeholder" dropdown: Bits UI `<DropdownMenu.Root>` with items for each token (`{{country}}`, `{{country_adjective}}`, `{{continent}}`, `{{diaspora_phrase}}`, `{{destination}}`). On select, append token at cursor position.
- Live preview: if `geoCtx`, show resolved string below textarea: `<p class="mt-1 text-xs text-[var(--color-muted)]">Preview: {resolveString(value, geoCtx.tokens)}</p>`
- Label: `<label class="text-xs font-medium text-[var(--color-muted)]">{label}</label>` above textarea

- [ ] **Step 2: Create editors index.ts**

```typescript
export { default as GeoText } from './GeoText.svelte';
```

- [ ] **Step 3: Commit**

```bash
git add packages/landing-blocks/src/editors/GeoText.svelte packages/landing-blocks/src/editors/index.ts
git commit -m "feat(landing-blocks): add GeoText editor — textarea with placeholder dropdown + live preview"
```

---

## Task 3: MediaPicker Wrapper

**Files:**
- Create: `packages/landing-blocks/src/editors/MediaPicker.svelte`

**Interfaces:**
- Produces: `<MediaPicker {value} bind:onchange {mediaPickerComponent} />` — wraps an injected media picker component

- [ ] **Step 1: Write the MediaPicker wrapper**

Create `packages/landing-blocks/src/editors/MediaPicker.svelte`:
- Props: `value?: MediaRef`, `onchange: (v: MediaRef) => void`, `mediaPickerComponent?: Component` (the admin's MediaPickerModal, injected via DI)
- State: `open` = false
- If `value`: show thumbnail `<img src={value.url}>` + filename + "Change" button + "Remove" button
- If no `value`: show "Select image" button
- On click: set `open = true`, render `{#if mediaPickerComponent}{@const Picker = mediaPickerComponent}<Picker bind:open onselect={(m) => onchange({ media_id: m.id, url: m.url, alt: m.filename })} />{/if}`
- This keeps the package free of admin-specific imports — the admin passes its own `MediaPickerModal` component

- [ ] **Step 2: Export from editors/index.ts**

```typescript
export { default as MediaPicker } from './MediaPicker.svelte';
```

- [ ] **Step 3: Commit**

```bash
git add packages/landing-blocks/src/editors/MediaPicker.svelte packages/landing-blocks/src/editors/index.ts
git commit -m "feat(landing-blocks): add MediaPicker wrapper — DI-based media selection"
```

---

## Task 4: IconPicker

**Files:**
- Create: `packages/landing-blocks/src/editors/IconPicker.svelte`

- [ ] **Step 1: Write the IconPicker**

Create `packages/landing-blocks/src/editors/IconPicker.svelte`:
- Props: `value?: string`, `onchange: (v: string) => void`
- State: `search` = '', `open` = false
- Uses Bits UI `<Popover.Root>` with `<Popover.Trigger>` (button showing current icon or "Select icon")
- Content: searchable grid of Lucide icons
- Import `* as LucideIcons from 'lucide-svelte'`
- `iconNames` = Object.keys(LucideIcons).filter(name => typeof LucideIcons[name] === 'function' || typeof LucideIcons[name] === 'object')
- `filteredIcons` derived from search query
- Grid: `{#each filteredIcons as name}` with `<button onclick={() => { onchange(name); open = false; }}><Icon size={20} /></button>`
- Search input at top of popover content
- Scrollable grid (max-h-64 overflow-y-auto)

- [ ] **Step 2: Export from editors/index.ts**

```typescript
export { default as IconPicker } from './IconPicker.svelte';
```

- [ ] **Step 3: Commit**

```bash
git add packages/landing-blocks/src/editors/IconPicker.svelte packages/landing-blocks/src/editors/index.ts
git commit -m "feat(landing-blocks): add IconPicker — searchable Lucide icon grid in Bits UI Popover"
```

---

## Task 5: BackgroundPicker

**Files:**
- Create: `packages/landing-blocks/src/editors/BackgroundPicker.svelte`

- [ ] **Step 1: Write the BackgroundPicker**

Create `packages/landing-blocks/src/editors/BackgroundPicker.svelte`:
- Props: `value?: BackgroundConfig`, `onchange: (v: BackgroundConfig) => void`, `mediaPickerComponent?: Component`
- Type selector: segmented control (none/color/gradient/image/video)
- Color: color input `<input type="color">` + text field for hex/var
- Gradient: two color inputs (from/to) + angle slider (0-360) + radial toggle
- Image: `<MediaPicker mediaPickerComponent={mediaPickerComponent}>` + fit selector (cover/contain/repeat/fixed) + overlay config (color + opacity slider)
- Video: `<MediaPicker mediaPickerComponent={mediaPickerComponent}>` + loop/muted/autoplay toggles + poster MediaPicker
- On any change, call `onchange` with updated BackgroundConfig

- [ ] **Step 2: Export from editors/index.ts**

```typescript
export { default as BackgroundPicker } from './BackgroundPicker.svelte';
```

- [ ] **Step 3: Commit**

```bash
git add packages/landing-blocks/src/editors/BackgroundPicker.svelte packages/landing-blocks/src/editors/index.ts
git commit -m "feat(landing-blocks): add BackgroundPicker — color/gradient/image/video selector"
```

---

## Task 6: CTAEditor

**Files:**
- Create: `packages/landing-blocks/src/editors/CTAEditor.svelte`

- [ ] **Step 1: Write the CTAEditor**

Create `packages/landing-blocks/src/editors/CTAEditor.svelte`:
- Props: `value: CTA`, `onchange: (v: CTA) => void`
- Fields: label (text input), href (text input), variant (select: primary/ghost/outline/link), icon (IconPicker)
- On any field change, call `onchange` with updated CTA
- Layout: vertical stack of labeled inputs

- [ ] **Step 2: Export from editors/index.ts**

```typescript
export { default as CTAEditor } from './CTAEditor.svelte';
```

- [ ] **Step 3: Commit**

```bash
git add packages/landing-blocks/src/editors/CTAEditor.svelte packages/landing-blocks/src/editors/index.ts
git commit -m "feat(landing-blocks): add CTAEditor — label, href, variant, icon"
```

---

## Task 7: EntityRefPicker

**Files:**
- Create: `packages/landing-blocks/src/editors/EntityRefPicker.svelte`

- [ ] **Step 1: Write the EntityRefPicker**

Create `packages/landing-blocks/src/editors/EntityRefPicker.svelte`:
- Props: `entityType: EntityType`, `value?: { id?: string; slug?: string }`, `onchange: (v: { id?: string; slug?: string }) => void`, `fetchCtx?: FetchContext`
- State: `search` = '', `results` = [], `loading` = false
- `$effect`: debounced search — calls `fetchEntityCollection(entityType, { filters: { q: search }, limit: 10 }, fetchCtx)` when search changes
- Renders: search input + dropdown of results (title + id/slug). On select, call `onchange({ id: result.id, slug: result.slug })`
- If `value` already set: show current selection with "Change" button

- [ ] **Step 2: Export from editors/index.ts**

```typescript
export { default as EntityRefPicker } from './EntityRefPicker.svelte';
```

- [ ] **Step 3: Commit**

```bash
git add packages/landing-blocks/src/editors/EntityRefPicker.svelte packages/landing-blocks/src/editors/index.ts
git commit -m "feat(landing-blocks): add EntityRefPicker — search entities by type"
```

---

## Task 8: NestedBlocksEditor

**Files:**
- Create: `packages/landing-blocks/src/editors/NestedBlocksEditor.svelte`

- [ ] **Step 1: Write the NestedBlocksEditor**

Create `packages/landing-blocks/src/editors/NestedBlocksEditor.svelte`:
- Props: `blocks: Block[]`, `onchange: (v: Block[]) => void`, `allowedTypes: Block['type'][]`, `mediaPickerComponent?: Component`, `geoCtx?: GeoContext`, `fetchCtx?: FetchContext`
- Mini version of the section list: ordered list with drag handle, block label, delete button
- "+ Add" button → palette picker (Bits UI Dialog) showing `allowedTypes`
- Drag reorder (HTML5 drag API or simple up/down buttons for MVP)
- Each nested block can be clicked to expand its content editor inline
- This is recursive — nested blocks can themselves contain nested blocks (Carousel slides, Tab content, Accordion items)

- [ ] **Step 2: Export from editors/index.ts**

```typescript
export { default as NestedBlocksEditor } from './NestedBlocksEditor.svelte';
```

- [ ] **Step 3: Commit**

```bash
git add packages/landing-blocks/src/editors/NestedBlocksEditor.svelte packages/landing-blocks/src/editors/index.ts
git commit -m "feat(landing-blocks): add NestedBlocksEditor — mini section list for nested blocks"
```

---

## Task 9: BlockStylePanel

**Files:**
- Create: `packages/landing-blocks/src/editors/BlockStylePanel.svelte`

**Interfaces:**
- Consumes: `BlockStyle` from `../types`
- Produces: `<BlockStylePanel {value} bind:onchange />` — grouped accordion controls for all style properties

- [ ] **Step 1: Write the BlockStylePanel**

Create `packages/landing-blocks/src/editors/BlockStylePanel.svelte`:
- Props: `value?: BlockStyle`, `onchange: (v: BlockStyle) => void`
- Uses Bits UI `<Accordion.Root>` with multiple expandable sections
- Groups (each an Accordion.Item):
  1. **Layout**: position (select), top/right/bottom/left (number inputs), z-index (select), width (select), display (select), flex_direction (select), align_items (select), justify_content (select), gap (select), grid_template_columns (number)
  2. **Typography**: heading_size (select), heading_weight (select 100-900), heading_family (select), heading_color (color input), body_size (select), body_weight (select), body_color (color input), text_align (select)
  3. **Color & Background**: text/heading/accent color inputs + `<BackgroundPicker>`
  4. **Border**: width (select), style (select), color (color input), radius (select), sides (select)
  5. **Spacing**: padding (select or per-side), margin (select or per-side)
  6. **Effects**: shadow (select), opacity (slider 0-100), backdrop_blur (select), overflow (select)
  7. **Transform**: scale (slider), rotate (slider -360 to 360), translate_x (number), translate_y (number)
  8. **Transition**: property (select), duration (number ms), easing (select), delay (number ms)
  9. **Icon** (when applicable): `<IconPicker>` + size (select), color (color input), background_color (color input), shape (select), position (select)
- Each group has a "Reset to default" button that clears that group's values
- On any field change, call `onchange` with updated BlockStyle (immutable update — spread + override)

- [ ] **Step 2: Export from editors/index.ts**

```typescript
export { default as BlockStylePanel } from './BlockStylePanel.svelte';
```

- [ ] **Step 3: Commit**

```bash
git add packages/landing-blocks/src/editors/BlockStylePanel.svelte packages/landing-blocks/src/editors/index.ts
git commit -m "feat(landing-blocks): add BlockStylePanel — shared Style tab with 9 grouped accordions"
```

---

## Task 10: Content Editors — Core Blocks

**Files:**
- Create: `packages/landing-blocks/src/editors/content/HeroContentEditor.svelte`
- Create: `packages/landing-blocks/src/editors/content/CtaBannerContentEditor.svelte`
- Create: `packages/landing-blocks/src/editors/content/CardGridContentEditor.svelte`
- Create: `packages/landing-blocks/src/editors/content/MediaGridContentEditor.svelte`
- Create: `packages/landing-blocks/src/editors/content/AccordionContentEditor.svelte`
- Create: `packages/landing-blocks/src/editors/content/CarouselContentEditor.svelte`
- Create: `packages/landing-blocks/src/editors/content/EntityCollectionContentEditor.svelte`
- Create: `packages/landing-blocks/src/editors/content/EntityFeatureContentEditor.svelte`

- [ ] **Step 1: Write HeroContentEditor**

Create `packages/landing-blocks/src/editors/content/HeroContentEditor.svelte`:
- Props: `props: HeroProps`, `onchange: (v: HeroProps) => void`, `mediaPickerComponent?: Component`, `geoCtx?: GeoContext`
- Fields: eyebrow (`<GeoText>`), headline (`<GeoText>`), subhead (`<GeoText>`), CTAs array (list of `<CTAEditor>` with add/remove), stats array (list of {label, value} with add/remove), floating_card toggle
- On any change, call `onchange` with updated props

- [ ] **Step 2: Write CtaBannerContentEditor**

Create `packages/landing-blocks/src/editors/content/CtaBannerContentEditor.svelte`:
- Props: `props: CtaBannerProps`, `onchange`, `geoCtx?`
- Fields: headline (`<GeoText>`), subhead (`<GeoText>`), CTAs array, alignment (select)

- [ ] **Step 3: Write CardGridContentEditor**

Create `packages/landing-blocks/src/editors/content/CardGridContentEditor.svelte`:
- Props: `props: CardGridProps`, `onchange`, `mediaPickerComponent?`, `geoCtx?`
- Fields: card_variant (select), layout (select), columns (slider 1-6), cards array (each card: icon via `<IconPicker>`, title `<GeoText>`, description `<GeoText>`, media via `<MediaPicker>`, CTA via `<CTAEditor>`, rating, stat_value, stat_label)

- [ ] **Step 4: Write MediaGridContentEditor**

Create `packages/landing-blocks/src/editors/content/MediaGridContentEditor.svelte`:
- Props: `props: MediaGridProps`, `onchange`, `mediaPickerComponent?`
- Fields: card_style (select), lightbox (toggle), columns (slider), items array (each: media via `<MediaPicker>`, caption, href)

- [ ] **Step 5: Write AccordionContentEditor**

Create `packages/landing-blocks/src/editors/content/AccordionContentEditor.svelte`:
- Props: `props: AccordionProps`, `onchange`, `mediaPickerComponent?`, `geoCtx?`, `fetchCtx?`
- Fields: preset (select), layout (select), searchable (toggle), items array (each: title `<GeoText>`, content_blocks via `<NestedBlocksEditor>`)

- [ ] **Step 6: Write CarouselContentEditor**

Create `packages/landing-blocks/src/editors/content/CarouselContentEditor.svelte`:
- Props: `props: CarouselProps`, `onchange`, `mediaPickerComponent?`, `geoCtx?`, `fetchCtx?`
- Fields: autoplay (toggle), dots (toggle), arrows (toggle), transition (select), interval (number), pause_on_hover (toggle), slides via `<NestedBlocksEditor>` with allowedTypes = all 23

- [ ] **Step 7: Write EntityCollectionContentEditor**

Create `packages/landing-blocks/src/editors/content/EntityCollectionContentEditor.svelte`:
- Props: `props: EntityCollectionProps`, `onchange`
- Fields: entity_type (select from EntityType union), layout (select), card_variant (select), columns (slider), query.filters (key-value editor), query.sort (select), query.limit (number), query.geo (toggle)

- [ ] **Step 8: Write EntityFeatureContentEditor**

Create `packages/landing-blocks/src/editors/content/EntityFeatureContentEditor.svelte`:
- Props: `props: EntityFeatureProps`, `onchange`, `fetchCtx?`, `mediaPickerComponent?`
- Fields: entity_type (select), layout (select), ref via `<EntityRefPicker>`, media_override via `<MediaPicker>`

- [ ] **Step 9: Commit all 8 content editors**

```bash
git add packages/landing-blocks/src/editors/content/
git commit -m "feat(landing-blocks): add 8 core block content editors"
```

---

## Task 11: Content Editors — Remaining 15 Blocks

**Files:**
- Create: 15 more content editors in `packages/landing-blocks/src/editors/content/`

- [ ] **Step 1: Write the remaining content editors**

Create one content editor per remaining block type. Each follows the same pattern: typed props, onchange callback, shared sub-editors where applicable.

- `VideoContentEditor.svelte`: provider (select), url/id (text), aspect_ratio (select), autoplay/mute/captions/loop (toggles), thumbnail (`<MediaPicker>`), title/description (`<GeoText>`)
- `IconContentEditor.svelte`: icon (`<IconPicker>`), size (select), color (color input), background_color (color input), shape (select), label (text), href (text)
- `ContentEditor.svelte`: html (textarea — TipTap editor integration in admin), prose_style (select)
- `QuoteContentEditor.svelte`: quote (`<GeoText>`), author (text), role (text), avatar (`<MediaPicker>`), layout (select)
- `SpacerDividerContentEditor.svelte`: height (slider), divider_style (select), divider_color (color input)
- `CountdownContentEditor.svelte`: target_date (datetime input), label (`<GeoText>`), layout (select), show_days/hours/minutes/seconds (toggles), on_expire (select), expired_message (`<GeoText>`)
- `ComingSoonContentEditor.svelte`: title (`<GeoText>`), description (`<GeoText>`), icon (`<IconPicker>`), estimated_date (date input), waitlist_form (toggle + fields), background (`<BackgroundPicker>`)
- `FormEmbedContentEditor.svelte`: preset (select), title/subtitle (`<GeoText>`), fields array (name, label, type, required, placeholder, options), submit_endpoint (text), submit_label (text), success_message (`<GeoText>`), error_message (`<GeoText>`)
- `TimelineContentEditor.svelte`: layout (select), steps array (icon `<IconPicker>`, title `<GeoText>`, description `<GeoText>`, media `<MediaPicker>`)
- `MapContentEditor.svelte`: mode (select), geo_source (select), highlight_visitor_country (toggle), pins array (country, label, href)
- `TableContentEditor.svelte`: columns array (key, label, align), rows array (key-value), sortable (toggle), striped (toggle)
- `AdvertisementContentEditor.svelte`: slot_key (select from ad slot keys)
- `LiveRateTickerContentEditor.svelte`: corridors (text array), limit (number), geo_aware (toggle), layout (select), speed (number)
- `TabsContentEditor.svelte`: layout (select), orientation (select), initial_tab (number), tabs array (label, icon `<IconPicker>`, content_blocks `<NestedBlocksEditor>`)
- `MediaContentEditor.svelte`: media (`<MediaPicker>`), layout (select), caption (`<GeoText>`), lightbox (toggle)

- [ ] **Step 2: Commit**

```bash
git add packages/landing-blocks/src/editors/content/
git commit -m "feat(landing-blocks): add 15 remaining block content editors"
```

---

## Task 12: Content Editor Registry

**Files:**
- Create: `packages/landing-blocks/src/editors/content/index.ts`

- [ ] **Step 1: Write the content editor registry**

Create `packages/landing-blocks/src/editors/content/index.ts`:
- Import all 23 content editors
- Export `contentEditorRegistry: Record<Block['type'], Component>` mapping each block type to its content editor component

```typescript
import type { Component } from 'svelte';
import type { Block } from '../../types';
import HeroContentEditor from './HeroContentEditor.svelte';
import CtaBannerContentEditor from './CtaBannerContentEditor.svelte';
// ... all 23

export const contentEditorRegistry: Record<Block['type'], Component> = {
  hero: HeroContentEditor,
  cta_banner: CtaBannerContentEditor,
  card_grid: CardGridContentEditor,
  // ... all 23
};
```

- [ ] **Step 2: Export from editors/index.ts**

```typescript
export { contentEditorRegistry } from './content/index.js';
```

- [ ] **Step 3: Commit**

```bash
git add packages/landing-blocks/src/editors/content/index.ts packages/landing-blocks/src/editors/index.ts
git commit -m "feat(landing-blocks): add content editor registry — type → editor component map"
```

---

## Task 13: Landing Pages Index

**Files:**
- Create: `apps/admin/src/routes/landing/+page.server.ts`
- Create: `apps/admin/src/routes/landing/+page.svelte`

- [ ] **Step 1: Write the server load**

Create `apps/admin/src/routes/landing/+page.server.ts`:
- Fetch `GET /admin/pages?page_type=landing` (or filter by page_type) via the admin API client
- Return `{ pages }` — list of landing pages with id, title, slug, page_type, status, updated_at

- [ ] **Step 2: Write the index page**

Create `apps/admin/src/routes/landing/+page.svelte`:
- Uses the existing DataTable component from `$lib/components/ui/data-table`
- Columns: title, slug, page_type, status (draft/published/archived badge), last_updated, actions (edit, duplicate, archive, view public)
- "Create landing page" button → opens a dialog to pick page type from `landingPalettes` → creates page via `POST /admin/pages` with the selected slug → routes to `/admin/landing/[slug]`
- Edit action → `goto('/admin/landing/' + page.slug)`
- Reuses the existing DataTable spec (sort, search, pagination)

- [ ] **Step 3: Commit**

```bash
git add apps/admin/src/routes/landing/+page.server.ts apps/admin/src/routes/landing/+page.svelte
git commit -m "feat(admin): add landing pages index DataTable"
```

---

## Task 14: Landing Page Editor — Server Load

**Files:**
- Create: `apps/admin/src/routes/landing/[slug]/+page.server.ts`

- [ ] **Step 1: Write the server load**

Create `apps/admin/src/routes/landing/[slug]/+page.server.ts`:
- Fetch `GET /admin/pages?slug={slug}` to find the page
- If not found: create a new page with `POST /admin/pages` using the slug + default seed from `landingPalettes[pageType]`
- Fetch `GET /admin/pagebuilder/themes` for theme selector
- Return `{ page, palette: landingPalettes[pageType], themes }`
- The page's `blocks` JSON is parsed into the `Block[]` type

- [ ] **Step 2: Commit**

```bash
git add apps/admin/src/routes/landing/[slug]/+page.server.ts
git commit -m "feat(admin): add landing page editor server load"
```

---

## Task 15: Landing Page Editor — Three-Pane UI

**Files:**
- Create: `apps/admin/src/routes/landing/[slug]/+page.svelte`

- [ ] **Step 1: Write the three-pane editor**

Create `apps/admin/src/routes/landing/[slug]/+page.svelte`:
- Props: `data: PageData` (page, palette, themes)
- State: `blocks` = $state(data.page.blocks), `selectedBlockId` = $state<string | null>(null), `previewCountry` = $state('global'), `unsavedChanges` = $state(false), `deviceMode` = $state<'desktop' | 'tablet' | 'mobile'>('desktop')
- `selectedBlock` derived from `blocks.find(b => b.id === selectedBlockId)`
- `previewCtx` derived: GeoContext with `getGeoTokens(previewCountry, 'en')`
- `fetchCtx` for entity fetchers in preview: constructed from admin API client

**Top bar:**
- Back link to `/admin/landing`
- Page title (editable)
- "Preview as: [Country]" dropdown (Bits UI Select with the 12 geo countries + Global)
- Device toggle: Desktop/Tablet/Mobile buttons
- "Save draft" button → `PATCH /admin/pages/{id}` with current blocks
- "Publish" button → save + `POST /admin/pages/{id}/publish` with confirmation dialog
- "Theme" dropdown (Bits UI Select from themes list)

**Left pane (section list):**
- Ordered list of `blocks` — each row: drag handle, block icon (from blockRegistry), editable label, type label, visibility toggle (eye icon), duplicate, delete, "⋯" menu (Replace with...)
- "+ Add section" button → Bits UI Dialog showing `palette.allowedBlocks` as cards (icon, label, description). On select, append new block with default props.
- Drag reorder: HTML5 drag API (draggable rows, dragover, drop) updating the blocks array
- Replace: "⋯" → "Replace with..." → palette picker filtered to compatible types. Swaps block type, carries over common props (style, label, visible).

**Center pane (live preview):**
- Renders `<LandingRenderer blocks={visibleBlocks}>` inside a scaled container
- `deviceWidth`: desktop=1280, tablet=768, mobile=375 — container scales to fit pane width
- Click any block → sets `selectedBlockId` → outline highlight on selected block
- Passes `geoCtx` and `fetchCtx` to LandingRenderer for geo-aware preview + entity data

**Right pane (inspector):**
- Three tabs: Content | Style | Settings (Bits UI Tabs)
- **Content tab**: looks up `contentEditorRegistry[selectedBlock.type]`, renders it with `props={selectedBlock.props}`, `onchange` updates the block's props in the blocks array, `mediaPickerComponent` = the admin's `MediaPickerModal`, `geoCtx` = previewCtx, `fetchCtx`
- **Style tab**: `<BlockStylePanel value={selectedBlock.style} onchange={(v) => updateBlockStyle(selectedBlock.id, v)}>`
- **Settings tab**: visibility toggle, motion config (preset select, delay number, stagger_children toggle), visibility rules (geo_countries multi-select, hide_if_empty toggle), block label (text input), lock toggle

**Unsaved indicator:**
- Any change sets `unsavedChanges = true`
- Dot indicator on changed sections
- "Save draft" clears it
- Browser beforeunload warning if unsaved

- [ ] **Step 2: Commit**

```bash
git add apps/admin/src/routes/landing/[slug]/+page.svelte
git commit -m "feat(admin): add three-pane landing page editor — section list, live preview, inspector"
```

---

## Task 16: Admin Nav Entry

**Files:**
- Modify: `apps/admin/src/routes/+layout.svelte`

- [ ] **Step 1: Add "Landing Pages" nav entry**

In the sidebar navigation (where other admin sections are listed), add a new entry:
- Label: "Landing Pages"
- Icon: LayoutTemplate (Lucide)
- href: `/admin/landing`
- Permission: `pagebuilder.blocks.manage` (same as existing page-builder routes)

Place it near the existing "Page Builder" entry — they're related but distinct (Page Builder = generic block management, Landing Pages = purpose-built landing page manager).

- [ ] **Step 2: Commit**

```bash
git add apps/admin/src/routes/+layout.svelte
git commit -m "feat(admin): add Landing Pages nav entry"
```

---

## Task 17: Save/Publish Actions

**Files:**
- Modify: `apps/admin/src/routes/landing/[slug]/+page.svelte`

- [ ] **Step 1: Implement save draft**

Add a `saveDraft()` function:
- `PATCH /admin/pages/{page.id}` with body `{ blocks: JSON.stringify(blocks) }`
- On success: `unsavedChanges = false`, show success toast
- On error: show error toast, keep unsavedChanges true
- Uses the admin API client (`apiPatch` or the extended `pageBuilderAdminApi`)

- [ ] **Step 2: Implement publish**

Add a `publish()` function:
- First call `saveDraft()` to persist current blocks
- Then `POST /admin/pages/{page.id}/publish`
- Show confirmation dialog before publishing (with diff summary: count of added/removed/reordered/edited sections)
- On success: show success toast, update page status to "published"

- [ ] **Step 3: Implement theme switch**

Add a `switchTheme(themeId)` function:
- `PATCH /admin/pages/{page.id}` with `{ theme_id: themeId }`
- Update local state

- [ ] **Step 4: Commit**

```bash
git add apps/admin/src/routes/landing/[slug]/+page.svelte
git commit -m "feat(admin): implement save draft, publish, and theme switch actions"
```

---

## Task 18: Block CRUD Operations

**Files:**
- Modify: `apps/admin/src/routes/landing/[slug]/+page.svelte`

- [ ] **Step 1: Implement block CRUD functions**

Add these functions to the editor:

```typescript
function addBlock(type: Block['type']): void {
  // Create new block with default props from palette or empty
  const newBlock: Block = { id: crypto.randomUUID(), type, visible: true, props: getDefaultProps(type) };
  blocks = [...blocks, newBlock];
  selectedBlockId = newBlock.id;
  unsavedChanges = true;
}

function removeBlock(id: string): void {
  blocks = blocks.filter(b => b.id !== id);
  if (selectedBlockId === id) selectedBlockId = null;
  unsavedChanges = true;
}

function duplicateBlock(id: string): void {
  const block = blocks.find(b => b.id === id);
  if (!block) return;
  const copy = { ...block, id: crypto.randomUUID(), label: (block.label ?? '') + ' (copy)' };
  const idx = blocks.findIndex(b => b.id === id);
  blocks = [...blocks.slice(0, idx + 1), copy, ...blocks.slice(idx + 1)];
  selectedBlockId = copy.id;
  unsavedChanges = true;
}

function toggleBlockVisibility(id: string): void {
  blocks = blocks.map(b => b.id === id ? { ...b, visible: !b.visible } : b);
  unsavedChanges = true;
}

function replaceBlock(id: string, newType: Block['type']): void {
  // Carry over common props: style, label, visible
  const block = blocks.find(b => b.id === id);
  if (!block) return;
  const replaced = { ...block, type: newType, props: getDefaultProps(newType) };
  blocks = blocks.map(b => b.id === id ? replaced : b);
  unsavedChanges = true;
}

function reorderBlocks(fromIdx: number, toIdx: number): void {
  const reordered = [...blocks];
  const [moved] = reordered.splice(fromIdx, 1);
  reordered.splice(toIdx, 0, moved);
  blocks = reordered;
  unsavedChanges = true;
}

function updateBlockProps(id: string, props: Record<string, unknown>): void {
  blocks = blocks.map(b => b.id === id ? { ...b, props: { ...b.props, ...props } } : b);
  unsavedChanges = true;
}

function updateBlockStyle(id: string, style: BlockStyle): void {
  blocks = blocks.map(b => b.id === id ? { ...b, style } : b);
  unsavedChanges = true;
}
```

- [ ] **Step 2: Commit**

```bash
git add apps/admin/src/routes/landing/[slug]/+page.svelte
git commit -m "feat(admin): implement block CRUD — add, remove, duplicate, toggle, replace, reorder, update"
```

---

## Task 19: Integration Verification

- [ ] **Step 1: Run svelte-check on apps/admin**

Run: `cd /home/jmobile/websites/japa/japaconnect/apps/admin && bunx svelte-check --tsconfig ./tsconfig.json 2>&1 | tail -20`
Expected: no errors

- [ ] **Step 2: Start admin dev server**

Run: `cd /home/jmobile/websites/japa/japaconnect/apps/admin && bun run dev &`
Expected: server starts on port 3001

- [ ] **Step 3: Verify landing pages index loads**

Run: `curl -s http://localhost:3001/admin/landing | head -20`
Expected: HTML renders (may redirect to login if not authenticated)

- [ ] **Step 4: Verify editor route loads**

Run: `curl -s http://localhost:3001/admin/landing/homepage | head -20`
Expected: HTML renders (may redirect to login)

- [ ] **Step 5: Kill dev server**

Run: `kill %1`

- [ ] **Step 6: Commit any fixes**

```bash
git add -A
git commit -m "fix: Plan 3 integration verification fixes"
```

---

## Self-Review Notes

**Spec coverage:** §7 Admin Landing Page Manager UI → Tasks 13-18. §7 left pane (section list, add/remove/replace/toggle/reorder) → Task 15, 18. §7 center pane (live preview, device toggle, preview-as geo) → Task 15. §7 right pane (Content/Style/Settings tabs) → Tasks 9-12, 15. §7 top bar (save/publish/theme/preview) → Tasks 15, 17. §7 landing pages index → Task 13. §7 permissions → Task 16. §8 editors directory → Tasks 2-12. §8 tree-shaking → conditional exports in package.json (Plan 1 Task 1). §8 MediaPicker DI → Task 3.

**Type consistency:** All content editors use the prop interfaces from Plan 1 Task 2 / Plan 2 Task 1. `BlockStylePanel` uses `BlockStyle` from Plan 1. `GeoText` uses `GeoContext` + `resolveString` from Plan 1. `contentEditorRegistry` maps `Block['type']` → Component, matching the 23 types from Plan 1+2. `landingPalettes` from Plan 1 Task 18 / Plan 2 Task 18 used in Tasks 13-14.

**No placeholders:** All tasks have concrete file paths, component structures, and function signatures. The content editors (Tasks 10-11) list every field for every block type explicitly.

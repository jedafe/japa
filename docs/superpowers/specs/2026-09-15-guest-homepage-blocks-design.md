# Design — Decouple `GuestHomeBlock` into Reusable `guest_*` Page Builder Blocks

**Date:** 2026-09-15  
**Author:** Devin (via user request)  
**Status:** Approved for implementation  
**JEOS refs:** §16 (Page Builder), §23 (Main site frontend), ADR-058 (shell), ADR-064 (geo-aware personalization), Rule 16 (no hardcoded frontend data)

## Goal

Convert the monolithic `GuestHomeBlock` into a family of reusable, dnd-kit sortable Page Builder blocks. Add a reusable `guest_footer` landing block. Keep `GuestHeader` and `GuestFooter` shell components unchanged and do not create a separate header/footer dnd editor. Every new section block must be fully editable in the Page Builder: titles, summaries, links, CTAs, card logos, and content. Card-list blocks must support both dynamic (data-driven) and static (manually curated) cards, with add/remove/reorder and inline dnd-kit sorting in the content editor.

## Decisions from clarification

- `GuestHeader` and `GuestFooter` remain shell components. No dnd-kit editor for them.
- `GuestHomeBlock` is split into a `guest_home` container preset and a set of reusable child blocks.
- The Hero is split into a `guest_hero` container with the following children:
  - `guest_hero_text` (eyebrow, headline, subhead, CTAs)
  - `guest_hero_image` (image carousel; supports `slides: MediaRef[]`)
  - `guest_hero_stats` (hero stat row)
  - `guest_hero_destinations` (floating "Top Destinations" card)
  - `guest_hero_community` (floating "Join X members" card)
- The floating destination and community cards are **siblings** inside `guest_hero`, not children of `guest_hero_image`. They are positioned with the existing `BlockStyle` panel.
- A new `guest_footer` landing block provides an editable, multi-column footer that can be placed on any landing page.
- All `guest_*` blocks can read the existing `GuestHomeData` context and fall back to their own `props` overrides.
- Legacy `guest_home` blocks continue to render until converted.

## New block catalog

All new blocks live in `@japaconnect/landing-blocks` and are grouped under category `guest` in the Page Builder palette.

| Block | Type | Purpose |
|---|---|---|
| `guest_home` | container | Auto-populates with the default guest homepage sections. Can be reordered like any `container`. |
| `guest_hero` | container | Two-column hero layout. Holds `guest_hero_*` children. |
| `guest_hero_text` | content | Eyebrow, headline, subhead, CTAs. |
| `guest_hero_image` | media | Hero image carousel. `slides: MediaRef[]`; single slide renders as a static image. |
| `guest_hero_stats` | data | Hero stat row. Reads from `GuestHomeData.stats`; props can override individual stats. |
| `guest_hero_destinations` | data | Floating "Top Destinations" card. Reads from `GuestHomeData.topDestinations`. |
| `guest_hero_community` | data | Floating "Join X members" card. Reads from the community stat in `GuestHomeData.stats`. |
| `guest_categories` | content | "Explore What Matters" category cards. Reads from `GuestHomeData.categories`. Supports `layout: 'grid' | 'slider'` and configurable `columns`. |
| `guest_opportunities` | data | Featured opportunity cards. Reads from `GuestHomeData.opportunities`. Supports `layout: 'grid' | 'slider'` and configurable `columns`. |
| `guest_stories` | data | Success story cards. Reads from `GuestHomeData.stories`. Supports `layout: 'grid' | 'slider'` and configurable `columns`. |
| `guest_tools` | content | Tool/resource cards. Reads from `GuestHomeData.tools`. Supports `layout: 'grid' | 'slider'` and configurable `columns`. |
| `guest_newsletter` | action | Newsletter form block. Editable endpoint, labels, success/error messages. |
| `guest_footer` | layout | Multi-column footer landing block. Editable logo, tagline, socials, columns, links. |

## Data flow

### `GuestHomeData` context

`LandingRenderer` currently receives `guestHome?: GuestHomeData` as a prop and passes it only to `guest_home`. This design introduces a Svelte context containing a `Writable<GuestHomeData | undefined>` store, set by `LandingRenderer`:

- `packages/landing-blocks/src/data/guestHomeContext.ts`
  - `setGuestHomeData(data: GuestHomeData | undefined)` — creates (or updates) the store in context.
  - `getGuestHomeData()` — returns the store; blocks subscribe with Svelte's `$` rune.

`LandingRenderer` creates the store with the initial `guestHome` prop and updates it in an `$effect` when the preview data changes, so all `guest_*` blocks react without remounting.

### Per-block behavior

- Blocks that consume `GuestHomeData` (stats, categories, opportunities, stories, tools, destinations, community) derive their primary data from context.
- Each of those blocks also carries `props` overrides (e.g., `guest_opportunities` can override the list of opportunities; `guest_tools` can override tool data; `guest_stories` can override story cards).
- Blocks that do not need external data (`guest_hero_text`, `guest_hero_image`, `guest_newsletter`, `guest_footer`) are fully prop-driven.
- For SSR and the Page Builder preview, the server already assembles `GuestHomeData` in `apps/main/src/routes/(app)/+page.server.ts` and the admin page editor already builds `guestHomePreview` in `apps/admin/src/routes/page-builder/pages/[id]/+page.svelte`. Both pass the data into `LandingRenderer`, which sets the context.

### Static vs. dynamic card lists

Card-list blocks (`guest_opportunities`, `guest_stories`, `guest_tools`, `guest_categories`, `guest_hero_stats`, `guest_hero_destinations`, `guest_hero_community`, and `guest_footer` columns) support two modes:

- **Dynamic (default):** the block reads its items from `GuestHomeData` context. The content editor can still override individual fields (logo, title, description, href) or the whole list.
- **Static:** the block reads its items from `props.items`. The content editor provides add/remove/reorder controls and lets the author edit every card field. Static mode is intended for fully curated landing pages where the admin wants explicit control over each card.

The content editor for each list block uses an embedded dnd-kit sortable list so the author can drag cards to reorder them, matching the mental model of the main Page Builder.

### Card-list layout and columns

The card-list blocks (`guest_categories`, `guest_opportunities`, `guest_stories`, `guest_tools`) each expose:

- `layout: 'grid' | 'slider'` (default `'grid'` for categories/tools, `'slider'` for opportunities/stories).
- `columns: number` (default `3` for grid, `1` for slider visible cards).
- `responsive_columns?: { tablet?: number; mobile?: number }` so the same block can render 1 column on mobile, 2–3 on tablet, and 3–4 on desktop (`columns` serves as the desktop value).
- `gap: 'none' | 'xs' | 'sm' | 'md' | 'lg' | 'xl'`.
- `card_style: 'default' | 'outlined' | 'filled'`.

The content editor for these blocks provides:

- A layout toggle (`Grid` / `Slider`).
- A columns stepper for the desktop view.
- Responsive column steppers for `tablet` and `mobile` (optional; unset values inherit the desktop `columns`).
- A gap selector.
- A card style selector.

This lets the author turn the "Explore What Matters" grid into a slider, reduce the tools grid to two columns, or show opportunities as a grid instead of a horizontal scroll.

## Block component architecture

### `guest_home` container

- Implemented by a new `GuestHomeContainerBlock.svelte` (or a specialized `ContainerBlock` wrapper).
- Props:
  - `layout`: `'stack'` (default).
  - `children: Block[]`.
  - `copy?: GuestHomeCopy` — optional legacy copy override for unconverted blocks.
- When `props.children` is empty, the component renders a default set of children in the canonical order:
  1. `guest_hero`
  2. `guest_categories`
  3. `guest_opportunities`
  4. `guest_stories`
  5. `guest_tools`
  6. `guest_newsletter`
  7. `guest_footer`
- The Page Builder treats it as a container, so children can be reordered, deleted, and added with the existing dnd-kit logic.

### `guest_hero` container

- Implemented by `GuestHeroContainerBlock.svelte`.
- Layout: two-column grid on desktop (text left, image right), stacking on mobile.
- The right column is `position: relative` so sibling floating cards can be absolutely positioned with `BlockStyle`.
- Default children:
  1. `guest_hero_text`
  2. `guest_hero_image`
  3. `guest_hero_stats`
  4. `guest_hero_destinations` (absolute over the image)
  5. `guest_hero_community` (absolute over the image)

### `guest_hero_image`

- Props:
  - `slides: MediaRef[]`
  - `autoplay?: boolean`
  - `interval?: number`
  - `dots?: boolean`
  - `arrows?: boolean`
- Renders a single image if one slide is provided; otherwise renders a carousel.
- The component is wrapped in a `position: relative` container so floating sibling blocks can overlay it.

### `guest_footer`

- Props:
  - `logo?: { light?: string; dark?: string }`
  - `tagline?: string`
  - `socials?: Array<{ icon: string; url: string; label: string }>`
  - `columns?: Array<{ title: string; links: Array<{ label: string; url: string; target?: string }> }>`
- Content editor supports add/remove/reorder columns and links, icon picker for socials, and media picker for logo.
- Visual style matches the existing `GuestFooter` but is rendered as a landing-page block, not the global shell footer.

## Content editors

Each new block gets a dedicated editor in `packages/landing-blocks/src/editors/content/guest/` and is registered in `contentEditorRegistry`.

| Editor | Editable fields |
|---|---|
| `GuestHomeContentEditor` | Reset to default sections; legacy copy migration hint. |
| `GuestHeroContainerContentEditor` | Layout, gap, background. |
| `GuestHeroTextContentEditor` | Eyebrow, headline prefix/accent, subhead, CTAs (label, href, variant, icon). |
| `GuestHeroImageContentEditor` | Slides (`MediaPicker` multi-select), autoplay, interval, dots, arrows. |
| `GuestHeroStatsContentEditor` | Data/static toggle, override stats (value, label, icon), dnd-kit card reordering. |
| `GuestHeroDestinationsContentEditor` | Title, see-all href, max items, dnd-kit reordering of destination cards. |
| `GuestHeroCommunityContentEditor` | Title, subtitle, CTA icon/href. |
| `GuestCategoriesContentEditor` | Title, subtitle, layout, columns, responsive columns, gap, card style, dnd-kit add/remove/reorder of category cards. |
| `GuestOpportunitiesContentEditor` | Title, view-all href, max items, layout, columns, responsive columns, gap, card style, dnd-kit add/remove/reorder of opportunity cards. |
| `GuestStoriesContentEditor` | Title, subtitle, read-more href, layout, columns, responsive columns, gap, card style, dnd-kit add/remove/reorder of story cards. |
| `GuestToolsContentEditor` | Title, subtitle, layout, columns, responsive columns, gap, card style, dnd-kit add/remove/reorder of tool cards. |
| `GuestNewsletterContentEditor` | Title, subtitle, endpoint, placeholder, CTA label, success/error messages. |
| `GuestFooterContentEditor` | Logo, tagline, socials, dnd-kit add/remove/reorder of columns and links. |

## Page Builder integration

### Block palette

- Add all `guest_*` blocks to `blockRegistry` in `packages/landing-blocks/src/render/blockRegistry.ts`.
- All new blocks have category `guest` so the palette groups them.

### `makeBlock` defaults

Update `apps/admin/src/routes/page-builder/pages/[id]/+page.svelte`:

- `guest_home` → create a container with the default section children.
- `guest_hero` → create a container with default hero children.
- `guest_hero_image` → create a block with one placeholder slide.
- `guest_footer` → create a block with one placeholder column.

### dnd-kit behavior

The existing dnd-kit drag-and-drop (`SortableSection`, `ContainerChildren`, `handleDragEnd`) already operates on the `Block[]` tree. Because `guest_home` and `guest_hero` are containers, their children are reorderable with no additional dnd-kit code. Non-container `guest_*` blocks can be dragged between top-level and container positions like any other block.

## Backward compatibility

### Legacy `guest_home` blocks

- Existing `guest_home` blocks with `props.copy`, `props.hero_image`, `props.logo`, and `props.newsletter_endpoint` continue to render via a legacy component.
- The legacy component is extracted from the current `GuestHomeBlock.svelte` into `packages/landing-blocks/src/blocks/GuestHomeLegacyBlock.svelte`.
- `blockRegistry['guest_home']` maps to the new container component. The new container detects legacy props and either:
  - Renders `GuestHomeLegacyBlock` inline when `props.copy` or `props.hero_image` is present and `props.children` is empty, or
  - Provides a one-click "Convert to editable sections" action in `GuestHomeContentEditor` that distributes legacy props into new child blocks.

### Default guest homepage fallback

`apps/main/src/routes/(app)/+page.svelte` can still emit a single `guest_home` block with empty props. The new container renders its default children, preserving the current fallback behavior.

## Files touched

### `@japaconnect/landing-blocks`

- `src/types.ts` — add `GuestHomeContainerProps`, `GuestHeroContainerProps`, `GuestHeroTextProps`, `GuestHeroImageProps`, `GuestHeroStatsProps`, `GuestHeroDestinationsProps`, `GuestHeroCommunityProps`, `GuestCategoriesProps`, `GuestOpportunitiesProps`, `GuestStoriesProps`, `GuestToolsProps`, `GuestNewsletterProps`, `GuestFooterProps`; add a shared `GuestCardListBaseProps` for `layout`, `columns`, `responsive_columns`, `gap`, and `card_style`; update `Block` union.
- `src/render/blockRegistry.ts` — register new blocks.
- `src/render/LandingRenderer.svelte` — set `GuestHomeData` context; remove `guest_home`-only `guestHome` prop pass.
- `src/shell/BlockShell.svelte` — no structural changes expected.
- `src/data/guestHomeContext.ts` — new context helpers.
- `src/index.ts` — export new types and context helpers.
- New blocks:
  - `src/blocks/guest/GuestHomeContainerBlock.svelte`
  - `src/blocks/guest/GuestHeroContainerBlock.svelte`
  - `src/blocks/guest/GuestHeroTextBlock.svelte`
  - `src/blocks/guest/GuestHeroImageBlock.svelte`
  - `src/blocks/guest/GuestHeroStatsBlock.svelte`
  - `src/blocks/guest/GuestHeroDestinationsBlock.svelte`
  - `src/blocks/guest/GuestHeroCommunityBlock.svelte`
  - `src/blocks/guest/GuestCategoriesBlock.svelte`
  - `src/blocks/guest/GuestOpportunitiesBlock.svelte`
  - `src/blocks/guest/GuestStoriesBlock.svelte`
  - `src/blocks/guest/GuestToolsBlock.svelte`
  - `src/blocks/guest/GuestNewsletterBlock.svelte`
  - `src/blocks/guest/GuestFooterBlock.svelte`
  - `src/blocks/GuestHomeLegacyBlock.svelte` (extracted from current `GuestHomeBlock.svelte`)
- New content editors:
  - `src/editors/content/guest/GuestHomeContentEditor.svelte`
  - `src/editors/content/guest/GuestHeroContainerContentEditor.svelte`
  - `src/editors/content/guest/GuestHeroTextContentEditor.svelte`
  - `src/editors/content/guest/GuestHeroImageContentEditor.svelte`
  - `src/editors/content/guest/GuestHeroStatsContentEditor.svelte`
  - `src/editors/content/guest/GuestHeroDestinationsContentEditor.svelte`
  - `src/editors/content/guest/GuestHeroCommunityContentEditor.svelte`
  - `src/editors/content/guest/GuestCategoriesContentEditor.svelte`
  - `src/editors/content/guest/GuestOpportunitiesContentEditor.svelte`
  - `src/editors/content/guest/GuestStoriesContentEditor.svelte`
  - `src/editors/content/guest/GuestToolsContentEditor.svelte`
  - `src/editors/content/guest/GuestNewsletterContentEditor.svelte`
  - `src/editors/content/guest/GuestFooterContentEditor.svelte`
- `src/editors/content/index.ts` — register new editors.

### `apps/admin`

- `src/routes/page-builder/pages/[id]/+page.svelte` — update `makeBlock()` for `guest_home`, `guest_hero`, and other `guest_*` defaults; ensure `guestHomePreview` still reaches `LandingRenderer`.
- `src/lib/components/landing-editor/ContainerChildren.svelte` — no structural changes; existing dnd-kit already handles the new container children.

### `apps/main`

- `src/routes/(app)/+page.svelte` — no change needed; the fallback `guest_home` block works with the new container.
- `src/routes/(app)/+page.server.ts` — no change needed; it already assembles `GuestHomeData`.

### JEOS

- `.jeos/sections/16-page-builder.md` — add the `guest_*` block catalog to the canonical block list.

## Testing plan

- Unit tests for `guestHomeContext` set/get.
- `LandingRenderer` tests that `guest_*` blocks can read `GuestHomeData` context.
- `GuestHeroImageBlock` tests for single image vs. carousel rendering.
- `GuestFooterBlock` tests for column rendering and empty-state fallbacks.
- Content editor tests for each new editor: form fields, onchange payloads, override arrays.
- Page Builder integration: `makeBlock('guest_home')` produces the default section children.
- Type check: `bun run check` in `packages/landing-blocks`, `apps/admin`, and `apps/main`.
- Build: `bun run build` in `packages/landing-blocks`, `apps/admin`, and `apps/main`.

## Open questions / none

All clarification questions resolved before this design was written.

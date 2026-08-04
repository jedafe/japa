# Modern Geo-Aware Homepage — Plan 2: Remaining 15 Block Primitives

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the 23-block catalog by adding the remaining 15 block primitives to `@japaconnect/landing-blocks`, expanding the entity fetchers for new entity types, and updating the block registry + homepage palette.

**Architecture:** Builds on Plan 1's foundation. Each new block follows the same pattern: typed props, `<BlockShell>` wrapper, `var(--color-*)` tokens, geo-aware copy, media-picker-compatible image fields, skeleton/error/empty states for data-bound blocks. New blocks are registered in `blockRegistry` and added to the homepage palette's `allowedBlocks`.

**Tech Stack:** SvelteKit 2, Svelte 5 Runes, Bits UI 2.18, Tailwind CSS v4, TypeScript (zero `any`), Vitest, lucide-svelte.

## Global Constraints

(Same as Plan 1 — see `docs/superpowers/plans/2026-08-04-modern-geo-aware-homepage-plan1.md` Global Constraints.)

## Spec Reference

`docs/superpowers/specs/2026-08-04-modern-geo-aware-homepage-design.md` — §3 (block catalog, full 23 primitives), §4 (style system), §5 (rendering), §9 (tool registry + entity types).

## Prerequisites

Plan 1 must be complete: `@japaconnect/landing-blocks` package exists with types, geo copy, style system, BlockShell, Reveal, blockRegistry, LandingRenderer, and the 8 core blocks.

---

## File Structure

### Create: `packages/landing-blocks/src/blocks/`

| File | Block type | Responsibility |
|---|---|---|
| `VideoBlock.svelte` | `video` | Embedded video (YouTube/Facebook/Vimeo/file) |
| `IconBlock.svelte` | `icon` | Pure icon display with full styling |
| `ContentBlock.svelte` | `content` | Rich text / prose (TipTap HTML) |
| `QuoteBlock.svelte` | `quote` | Single pull quote |
| `SpacerDividerBlock.svelte` | `spacer_divider` | Spacing + divider |
| `CountdownBlock.svelte` | `countdown` | Countdown timer |
| `ComingSoonBlock.svelte` | `coming_soon` | Teaser / waitlist |
| `FormEmbedBlock.svelte` | `form_embed` | Generic form (newsletter/waitlist/contact) |
| `TimelineBlock.svelte` | `timeline` | Sequential steps (japa journey) |
| `MapBlock.svelte` | `map` | Geo signature — world map with diaspora pins |
| `TableBlock.svelte` | `table` | Generic data table |
| `AdvertisementBlock.svelte` | `advertisement` | Ad slot |
| `LiveRateTickerBlock.svelte` | `live_rate_ticker` | Live remittance rate ticker |
| `TabsBlock.svelte` | `tabs` | Tabs of nested blocks |
| `MediaBlock.svelte` | `media` | Single media (image or self-hosted video) |

### Modify

| File | Change |
|---|---|
| `src/types.ts` | Add 15 new block types to the `Block` discriminated union + their prop interfaces |
| `src/render/blockRegistry.ts` | Register all 15 new blocks |
| `src/data/entityFetchers.ts` | Add `remittance_rate` fetcher for LiveRateTicker |
| `src/palettes/landingPalettes.ts` | Add new blocks to `allowedBlocks` + update `defaultSeed` |
| `src/index.ts` | Export new types |

---

## Task 1: Expand Types for 15 New Blocks

**Files:**
- Modify: `packages/landing-blocks/src/types.ts`
- Modify: `packages/landing-blocks/src/index.ts`

**Interfaces:**
- Produces: 15 new prop interfaces + expanded `Block` discriminated union

- [ ] **Step 1: Add 15 new prop interfaces to types.ts**

Add these interfaces after the existing prop interfaces:

```typescript
export interface VideoProps {
  provider: 'youtube' | 'facebook' | 'vimeo' | 'file';
  url?: string;
  id?: string;
  aspect_ratio: '16:9' | '4:3' | '1:1' | '21:9';
  autoplay: boolean;
  mute: boolean;
  captions: boolean;
  loop: boolean;
  thumbnail?: MediaRef;
  title?: string;
  description?: string;
}

export interface IconProps {
  icon: string;
  size: 'xs' | 'sm' | 'md' | 'lg' | 'xl' | '2xl' | number;
  color: string;
  background_color: string;
  shape: 'none' | 'circle' | 'square' | 'rounded';
  label?: string;
  href?: string;
}

export interface ContentProps {
  html: string;
  prose_style: 'default' | 'minimal' | 'article' | 'documentation';
}

export interface QuoteProps {
  quote: string;
  author: string;
  role?: string;
  avatar?: MediaRef;
  layout: 'centered' | 'inline' | 'large';
}

export interface SpacerDividerProps {
  height: number;
  divider_style: 'none' | 'line' | 'gradient' | 'dots';
  divider_color?: string;
}

export interface CountdownProps {
  target_date: string;
  label?: string;
  layout: 'inline' | 'stacked' | 'card';
  show_days: boolean;
  show_hours: boolean;
  show_minutes: boolean;
  show_seconds: boolean;
  on_expire: 'hide' | 'show_message';
  expired_message?: string;
}

export interface ComingSoonProps {
  title: string;
  description?: string;
  icon?: string;
  estimated_date?: string;
  waitlist_form?: { enabled: boolean; cta_label: string; endpoint: string };
  background?: BackgroundConfig;
}

export interface FormEmbedProps {
  preset: 'newsletter' | 'waitlist' | 'contact' | 'custom';
  fields: Array<{ name: string; label: string; type: 'text' | 'email' | 'textarea' | 'select'; required: boolean; placeholder?: string; options?: string[] }>;
  submit_endpoint: string;
  submit_label: string;
  success_message: string;
  error_message: string;
  title?: string;
  subtitle?: string;
}

export interface TimelineProps {
  steps: Array<{ icon?: string; title: string; description?: string; media?: MediaRef }>;
  layout: 'horizontal' | 'vertical' | 'zigzag';
}

export interface MapProps {
  mode: 'static' | 'dynamic';
  pins: Array<{ country: string; label: string; href?: string }>;
  geo_source: 'manual' | 'trending_spaces';
  highlight_visitor_country: boolean;
}

export interface TableProps {
  columns: Array<{ key: string; label: string; align?: 'left' | 'center' | 'right' }>;
  rows: Array<Record<string, string>>;
  sortable: boolean;
  striped: boolean;
}

export interface AdvertisementProps {
  slot_key: string;
}

export interface LiveRateTickerProps {
  corridors?: string[];
  limit: number;
  geo_aware: boolean;
  layout: 'horizontal' | 'vertical';
  speed?: number;
}

export interface TabsProps {
  tabs: Array<{ label: string; icon?: string; content_blocks?: Block[] }>;
  layout: 'underline' | 'pills' | 'boxed';
  orientation: 'horizontal' | 'vertical';
  initial_tab: number;
}

export interface MediaBlockProps {
  media: MediaRef;
  layout: 'full' | 'contained' | 'split';
  caption?: string;
  lightbox: boolean;
}
```

- [ ] **Step 2: Expand the Block discriminated union**

Add 15 new branches to the `Block` type:

```typescript
export type Block =
  | (BaseBlock & { type: 'hero'; props: HeroProps })
  | (BaseBlock & { type: 'cta_banner'; props: CtaBannerProps })
  | (BaseBlock & { type: 'card_grid'; props: CardGridProps })
  | (BaseBlock & { type: 'media_grid'; props: MediaGridProps })
  | (BaseBlock & { type: 'accordion'; props: AccordionProps })
  | (BaseBlock & { type: 'carousel'; props: CarouselProps })
  | (BaseBlock & { type: 'entity_collection'; props: EntityCollectionProps })
  | (BaseBlock & { type: 'entity_feature'; props: EntityFeatureProps })
  | (BaseBlock & { type: 'video'; props: VideoProps })
  | (BaseBlock & { type: 'icon'; props: IconProps })
  | (BaseBlock & { type: 'content'; props: ContentProps })
  | (BaseBlock & { type: 'quote'; props: QuoteProps })
  | (BaseBlock & { type: 'spacer_divider'; props: SpacerDividerProps })
  | (BaseBlock & { type: 'countdown'; props: CountdownProps })
  | (BaseBlock & { type: 'coming_soon'; props: ComingSoonProps })
  | (BaseBlock & { type: 'form_embed'; props: FormEmbedProps })
  | (BaseBlock & { type: 'timeline'; props: TimelineProps })
  | (BaseBlock & { type: 'map'; props: MapProps })
  | (BaseBlock & { type: 'table'; props: TableProps })
  | (BaseBlock & { type: 'advertisement'; props: AdvertisementProps })
  | (BaseBlock & { type: 'live_rate_ticker'; props: LiveRateTickerProps })
  | (BaseBlock & { type: 'tabs'; props: TabsProps })
  | (BaseBlock & { type: 'media'; props: MediaBlockProps });
```

- [ ] **Step 3: Export new types from index.ts**

Add to the existing type export:
```typescript
export type {
  ...existing...,
  VideoProps, IconProps, ContentProps, QuoteProps, SpacerDividerProps,
  CountdownProps, ComingSoonProps, FormEmbedProps, TimelineProps,
  MapProps, TableProps, AdvertisementProps, LiveRateTickerProps,
  TabsProps, MediaBlockProps,
} from './types';
```

- [ ] **Step 4: Verify types compile**

Run: `cd /home/jmobile/websites/japa/japaconnect/apps/main && bunx svelte-check --tsconfig ./tsconfig.json 2>&1 | tail -5`
Expected: no errors

- [ ] **Step 5: Commit**

```bash
git add packages/landing-blocks/src/types.ts packages/landing-blocks/src/index.ts
git commit -m "feat(landing-blocks): expand Block union with 15 new block prop types"
```

---

## Task 2: VideoBlock

**Files:**
- Create: `packages/landing-blocks/src/blocks/VideoBlock.svelte`

- [ ] **Step 1: Write the VideoBlock**

Create `packages/landing-blocks/src/blocks/VideoBlock.svelte`:
- Props: `props: VideoProps`
- `aspectClass` derived from aspect_ratio (16:9 → 'aspect-video', 4:3 → 'aspect-[4/3]', 1:1 → 'aspect-square', 21:9 → 'aspect-[21/9]')
- Provider rendering:
  - `youtube`: `<iframe src="https://www.youtube.com/embed/{id}" allow="autoplay; encrypted-media" allowfullscreen>`
  - `vimeo`: `<iframe src="https://player.vimeo.com/video/{id}" allowfullscreen>`
  - `facebook`: `<iframe src="https://www.facebook.com/plugins/video.php?href={encodeURIComponent(url)}" scrolling="no" allowfullscreen>`
  - `file`: `<video src={url} controls autoplay={props.autoplay} muted={props.mute} loop={props.loop}>`
- Thumbnail: if `props.thumbnail`, show `<img>` with play button overlay; click reveals iframe
- Title/description: optional `<h3>` + `<p>` below video
- Container: `<section class="px-6 py-16"><div class="mx-auto max-w-4xl">` with rounded overflow-hidden border

- [ ] **Step 2: Commit**

```bash
git add packages/landing-blocks/src/blocks/VideoBlock.svelte
git commit -m "feat(landing-blocks): add VideoBlock — YouTube/Facebook/Vimeo/file embed with thumbnail"
```

---

## Task 3: IconBlock

**Files:**
- Create: `packages/landing-blocks/src/blocks/IconBlock.svelte`

- [ ] **Step 1: Write the IconBlock**

Create `packages/landing-blocks/src/blocks/IconBlock.svelte`:
- Props: `props: IconProps`
- Import `* as LucideIcons from 'lucide-svelte'`
- `getIcon(name)` helper resolves Lucide icon by name
- `sizeMap`: xs=16, sm=20, md=24, lg=32, xl=48, 2xl=64 (or number directly)
- `shapeClass`: none → '', circle → 'rounded-full', square → 'rounded-none', rounded → 'rounded-[var(--radius)]'
- Container: `<section class="px-6 py-8"><div class="mx-auto max-w-6xl flex items-center justify-center gap-3">`
- Icon wrapper: `<div class="inline-flex {shapeClass} p-{padding}" style="background-color: {props.background_color}; color: {props.color}">`
- If `href`: wrap in `<a>`, else `<div>`
- If `label`: `<span class="text-sm font-medium" style="color: {props.color}">{props.label}</span>`

- [ ] **Step 2: Commit**

```bash
git add packages/landing-blocks/src/blocks/IconBlock.svelte
git commit -m "feat(landing-blocks): add IconBlock — configurable icon with size, color, background, shape"
```

---

## Task 4: ContentBlock

**Files:**
- Create: `packages/landing-blocks/src/blocks/ContentBlock.svelte`

- [ ] **Step 1: Write the ContentBlock**

Create `packages/landing-blocks/src/blocks/ContentBlock.svelte`:
- Props: `props: ContentProps`
- `proseClass` derived from prose_style: default → 'prose prose-lg max-w-none dark:prose-invert', minimal → 'prose prose-sm max-w-none', article → 'prose prose-lg max-w-3xl mx-auto', documentation → 'prose prose-sm max-w-4xl mx-auto'
- Renders: `<section class="px-6 py-16"><div class="mx-auto max-w-4xl"><div class="{proseClass}">{@html props.html}</div></div></section>`
- Note: `{@html}` is safe here because content comes from the admin TipTap editor (trusted source), not user-generated input

- [ ] **Step 2: Commit**

```bash
git add packages/landing-blocks/src/blocks/ContentBlock.svelte
git commit -m "feat(landing-blocks): add ContentBlock — rich text/prose with TipTap HTML"
```

---

## Task 5: QuoteBlock

**Files:**
- Create: `packages/landing-blocks/src/blocks/QuoteBlock.svelte`

- [ ] **Step 1: Write the QuoteBlock**

Create `packages/landing-blocks/src/blocks/QuoteBlock.svelte`:
- Props: `props: QuoteProps`
- `layoutClass`: centered → 'text-center mx-auto max-w-3xl', inline → 'max-w-4xl', large → 'text-center mx-auto max-w-5xl'
- Large quotation mark: `<span class="text-6xl text-[var(--color-primary)] font-serif">"</span>`
- Quote: `<blockquote class="text-xl text-[var(--color-text)] font-serif italic">`
- Author: avatar (if present) + name + role in a flex row
- Container: `<section class="px-6 py-16"><div class="{layoutClass}">`

- [ ] **Step 2: Commit**

```bash
git add packages/landing-blocks/src/blocks/QuoteBlock.svelte
git commit -m "feat(landing-blocks): add QuoteBlock — pull quote with avatar, author, role"
```

---

## Task 6: SpacerDividerBlock

**Files:**
- Create: `packages/landing-blocks/src/blocks/SpacerDividerBlock.svelte`

- [ ] **Step 1: Write the SpacerDividerBlock**

Create `packages/landing-blocks/src/blocks/SpacerDividerBlock.svelte`:
- Props: `props: SpacerDividerProps`
- Height: `style="height: {props.height}px"`
- Divider styles:
  - `none`: empty div with just height
  - `line`: `<hr class="border-t border-[var(--color-border)]">` centered with max-w
  - `gradient`: `<div style="height: 1px; background: linear-gradient(to right, transparent, var(--color-border), transparent)">`
  - `dots`: three centered dots `· · ·` in muted color
- Container: `<section style="height: {props.height}px" class="flex items-center justify-center">`

- [ ] **Step 2: Commit**

```bash
git add packages/landing-blocks/src/blocks/SpacerDividerBlock.svelte
git commit -m "feat(landing-blocks): add SpacerDividerBlock — spacing + line/gradient/dots divider"
```

---

## Task 7: CountdownBlock

**Files:**
- Create: `packages/landing-blocks/src/blocks/CountdownBlock.svelte`

- [ ] **Step 1: Write the CountdownBlock**

Create `packages/landing-blocks/src/blocks/CountdownBlock.svelte`:
- Props: `props: CountdownProps`
- State: `timeLeft` = `{ days, hours, minutes, seconds }`, `expired` = boolean
- `$effect`: `setInterval` every 1000ms, calculates diff from `props.target_date`, updates timeLeft, sets expired when diff ≤ 0, clears interval on expiry/unmount
- `layoutClass`: inline → 'flex flex-row gap-4', stacked → 'flex flex-col gap-2', card → 'grid grid-cols-4 gap-4'
- Each unit: `<div class="flex flex-col items-center rounded-[var(--radius)] bg-[var(--color-surface)] border border-[var(--color-border)] p-4"><span class="text-3xl font-bold">{value}</span><span class="text-xs text-[var(--color-muted)]">{label}</span></div>`
- Only show units where `show_days`/`show_hours`/`show_minutes`/`show_seconds` is true
- On expire: if `on_expire === 'hide'`, render nothing; if `show_message`, show `expired_message`
- Label: optional `<p class="mb-4 text-sm text-[var(--color-muted)]">{props.label}</p>` above countdown
- `prefers-reduced-motion`: show final value (0s) without the rAF count-down animation (the interval still runs but no visual animation)

- [ ] **Step 2: Commit**

```bash
git add packages/landing-blocks/src/blocks/CountdownBlock.svelte
git commit -m "feat(landing-blocks): add CountdownBlock — live timer with inline/stacked/card layouts"
```

---

## Task 8: ComingSoonBlock

**Files:**
- Create: `packages/landing-blocks/src/blocks/ComingSoonBlock.svelte`

- [ ] **Step 1: Write the ComingSoonBlock**

Create `packages/landing-blocks/src/blocks/ComingSoonBlock.svelte`:
- Props: `props: ComingSoonProps`
- Import `* as LucideIcons from 'lucide-svelte'` for icon resolution
- Layout: centered card with optional background (gradient/media)
- Icon: large Lucide icon in primary color circle
- Title: `<h2 class="text-2xl font-bold">`
- Description: `<p class="text-[var(--color-text-subtle)]">`
- Estimated date: `<p class="text-sm text-[var(--color-muted)]">Expected: {date}</p>`
- Waitlist form: if `waitlist_form.enabled`, render email input + submit button. On submit, POST to `waitlist_form.endpoint`. Show success/error states.
- Container: `<section class="px-6 py-16"><div class="mx-auto max-w-2xl rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-12 text-center">`

- [ ] **Step 2: Commit**

```bash
git add packages/landing-blocks/src/blocks/ComingSoonBlock.svelte
git commit -m "feat(landing-blocks): add ComingSoonBlock — teaser with optional waitlist form"
```

---

## Task 9: FormEmbedBlock

**Files:**
- Create: `packages/landing-blocks/src/blocks/FormEmbedBlock.svelte`

- [ ] **Step 1: Write the FormEmbedBlock**

Create `packages/landing-blocks/src/blocks/FormEmbedBlock.svelte`:
- Props: `props: FormEmbedProps`
- State: `formData` = Record<string, string>, `submitting` = boolean, `success` = boolean, `error` = boolean
- Renders form based on `fields` array: text → `<input type="text">`, email → `<input type="email">`, textarea → `<textarea>`, select → `<select>` with options
- Each field: label + input with `var(--color-*)` styling, required indicator
- Submit button: `<button type="submit" class="...primary...">` with `props.submit_label`
- On submit: `e.preventDefault()`, set submitting, POST to `props.submit_endpoint` with formData, show success_message or error_message
- Newsletter preset: single email field, posts to `/newsletter/subscribe`
- Waitlist preset: email field, posts to configured endpoint
- Container: `<section class="px-6 py-16"><div class="mx-auto max-w-xl rounded-[var(--radius-card)] bg-[var(--color-surface)] border border-[var(--color-border)] p-8">`

- [ ] **Step 2: Commit**

```bash
git add packages/landing-blocks/src/blocks/FormEmbedBlock.svelte
git commit -m "feat(landing-blocks): add FormEmbedBlock — generic form with newsletter/waitlist/contact presets"
```

---

## Task 10: TimelineBlock

**Files:**
- Create: `packages/landing-blocks/src/blocks/TimelineBlock.svelte`

- [ ] **Step 1: Write the TimelineBlock**

Create `packages/landing-blocks/src/blocks/TimelineBlock.svelte`:
- Props: `props: TimelineProps`
- Import `* as LucideIcons from 'lucide-svelte'`
- `getIcon(name)` helper
- Layout: `horizontal` → flex row with connecting line, `vertical` → flex col with left line, `zigzag` → alternating left/right
- Each step: numbered marker (01, 02, 03 — earned because the japa journey is a real ordered process), icon in circle, title, description, optional media
- Connecting line: `<div class="absolute top-1/2 left-0 right-0 h-0.5 bg-[var(--color-border)]">` (horizontal) or vertical equivalent
- Step card: `<div class="relative z-10 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6">`
- Number: `<span class="text-xs font-bold text-[var(--color-primary)]">0{i+1}</span>`
- Container: `<section class="px-6 py-16"><div class="mx-auto max-w-4xl">`

- [ ] **Step 2: Commit**

```bash
git add packages/landing-blocks/src/blocks/TimelineBlock.svelte
git commit -m "feat(landing-blocks): add TimelineBlock — japa journey steps with numbered markers"
```

---

## Task 11: MapBlock

**Files:**
- Create: `packages/landing-blocks/src/blocks/MapBlock.svelte`

- [ ] **Step 1: Write the MapBlock**

Create `packages/landing-blocks/src/blocks/MapBlock.svelte`:
- Props: `props: MapProps`, `geoCountry?: string`
- State: `pins` = props.pins (or fetched from `/trending/spaces?geo=` if `geo_source === 'trending_spaces'`)
- `$effect`: if `geo_source === 'trending_spaces'` and fetchCtx provided, fetch trending spaces and map to pins
- Static map: a stylized SVG world map (simplified continent outlines) with pulsing pin markers positioned by country
- Pin: `<div class="absolute" style="top: {y}%; left: {x}%"><div class="h-3 w-3 rounded-full bg-[var(--color-primary)] animate-pulse"></div><div class="mt-1 text-xs text-[var(--color-muted)]">{label}</div></div>`
- `highlight_visitor_country`: if `geoCountry` matches a pin's country, make it larger + primary color + ring
- Country → x/y position map: a `countryPositions` Record<ISO-2, {x, y}> with approximate world map coordinates for the top diaspora destinations
- Container: `<section class="px-6 py-16"><div class="mx-auto max-w-5xl rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-8"><div class="relative aspect-[2/1]">`

- [ ] **Step 2: Commit**

```bash
git add packages/landing-blocks/src/blocks/MapBlock.svelte
git commit -m "feat(landing-blocks): add MapBlock — geo signature with pulsing diaspora pins"
```

---

## Task 12: TableBlock

**Files:**
- Create: `packages/landing-blocks/src/blocks/TableBlock.svelte`

- [ ] **Step 1: Write the TableBlock**

Create `packages/landing-blocks/src/blocks/TableBlock.svelte`:
- Props: `props: TableProps`
- State: `sortKey`, `sortDir` (if sortable)
- `sortedRows` derived: if sortKey set, sort rows by that column
- Renders: `<table class="w-full border-collapse">`
- Header: `<thead><tr>` with `<th>` for each column, align class, sortable click handler
- Body: `<tbody>` with `<tr>` for each row, striped class on even rows
- Cell: `<td class="px-4 py-3 text-sm border-b border-[var(--color-border)]">`
- Container: `<section class="px-6 py-16"><div class="mx-auto max-w-4xl overflow-x-auto rounded-[var(--radius-card)] border border-[var(--color-border)]">`

- [ ] **Step 2: Commit**

```bash
git add packages/landing-blocks/src/blocks/TableBlock.svelte
git commit -m "feat(landing-blocks): add TableBlock — sortable striped data table"
```

---

## Task 13: AdvertisementBlock

**Files:**
- Create: `packages/landing-blocks/src/blocks/AdvertisementBlock.svelte`

- [ ] **Step 1: Write the AdvertisementBlock**

Create `packages/landing-blocks/src/blocks/AdvertisementBlock.svelte`:
- Props: `props: AdvertisementProps`
- Renders an ad slot container keyed by `props.slot_key`
- The actual ad content is injected by the ad system (existing ad slots: `hero_below`, `landing_inline_1`, `landing_inline_2`, `landing_footer`)
- Placeholder: `<div data-ad-slot={props.slot_key} class="ad-container min-h-[90px] flex items-center justify-center text-[var(--color-muted)] text-sm">Advertisement</div>`
- Container: `<section class="px-6 py-4">`
- Note: this is a slot marker — the ad rendering system (existing in apps/main) fills it client-side or via SSR ad injection

- [ ] **Step 2: Commit**

```bash
git add packages/landing-blocks/src/blocks/AdvertisementBlock.svelte
git commit -m "feat(landing-blocks): add AdvertisementBlock — ad slot marker for landing pages"
```

---

## Task 14: LiveRateTickerBlock

**Files:**
- Create: `packages/landing-blocks/src/blocks/LiveRateTickerBlock.svelte`
- Modify: `packages/landing-blocks/src/data/entityFetchers.ts`

- [ ] **Step 1: Add remittance rate fetcher to entityFetchers.ts**

Add a dedicated fetcher for the ticker (separate from the generic `fetchEntityCollection` because the ticker needs corridor filtering):

```typescript
export async function fetchRemittanceRates(
  ctx: FetchContext,
  corridors?: string[],
  limit: number = 10
): Promise<RemittanceRate[]> {
  const params = new URLSearchParams();
  if (corridors) for (const c of corridors) params.append('corridor', c);
  params.set('limit', String(limit));
  const headers: Record<string, string> = {};
  if (ctx.token) headers.authorization = `Bearer ${ctx.token}`;
  if (ctx.host) { headers.host = ctx.host; headers['x-forwarded-host'] = ctx.host; }
  try {
    const res = await ctx.fetch(`${ctx.apiBase}/api/v1/remittance/rates?${params}`, { headers });
    if (!res.ok) return [];
    const json = await res.json();
    return (json.data ?? json) as RemittanceRate[];
  } catch {
    return [];
  }
}
```

Export it from index.ts.

- [ ] **Step 2: Write the LiveRateTickerBlock**

Create `packages/landing-blocks/src/blocks/LiveRateTickerBlock.svelte`:
- Props: `props: LiveRateTickerProps`, `fetchCtx?: FetchContext`, `geoCountry?: string`
- State: `rates` = RemittanceRate[], `loading`, `error`
- `$effect`: calls `fetchRemittanceRates(fetchCtx, props.corridors, props.limit)`
- Layout: `horizontal` → marquee scroll (CSS keyframes), `vertical` → stacked list
- Each rate: `<div class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2"><span class="text-sm font-medium">{corridor}</span><span class="text-sm text-[var(--color-primary)]">{rate}</span></div>`
- Geo-aware: if `geo_aware` and `geoCountry`, filter corridors that include the visitor's country
- Marquee: CSS `@keyframes marquee { 0% { transform: translateX(0); } 100% { transform: translateX(-50%); } }` with `animation: marquee {speed}s linear infinite`, pauses on hover
- Skeleton: gray pill placeholders while loading
- Container: `<section class="px-6 py-4"><div class="mx-auto max-w-6xl overflow-hidden">`

- [ ] **Step 3: Export fetchRemittanceRates from index.ts**

```typescript
export { fetchRemittanceRates } from './data/entityFetchers';
```

- [ ] **Step 4: Commit**

```bash
git add packages/landing-blocks/src/blocks/LiveRateTickerBlock.svelte packages/landing-blocks/src/data/entityFetchers.ts packages/landing-blocks/src/index.ts
git commit -m "feat(landing-blocks): add LiveRateTickerBlock — geo-aware remittance rate marquee"
```

---

## Task 15: TabsBlock

**Files:**
- Create: `packages/landing-blocks/src/blocks/TabsBlock.svelte`

- [ ] **Step 1: Write the TabsBlock**

Create `packages/landing-blocks/src/blocks/TabsBlock.svelte`:
- Props: `props: TabsProps`
- Import `* as LucideIcons from 'lucide-svelte'`, `blockRegistry` from `../render/blockRegistry`, `BlockShell` from `../shell/BlockShell.svelte`
- State: `activeTab` = `props.initial_tab`
- Uses Bits UI `<Tabs.Root>` with `<Tabs.List>`, `<Tabs.Trigger>`, `<Tabs.Content>`
- `layoutClass`: underline → 'border-b border-[var(--color-border)]', pills → 'gap-2', boxed → 'gap-0 border border-[var(--color-border)] rounded-[var(--radius)]'
- Each tab trigger: label + optional icon (Lucide)
- Each tab content: renders nested `content_blocks` via `blockRegistry` dispatch + `BlockShell`
- `orientation`: horizontal → default, vertical → `Tabs.Root orientation="vertical"` with flex layout
- Container: `<section class="px-6 py-16"><div class="mx-auto max-w-5xl">`

- [ ] **Step 2: Commit**

```bash
git add packages/landing-blocks/src/blocks/TabsBlock.svelte
git commit -m "feat(landing-blocks): add TabsBlock — Bits UI tabs with nested block content"
```

---

## Task 16: MediaBlock

**Files:**
- Create: `packages/landing-blocks/src/blocks/MediaBlock.svelte`

- [ ] **Step 1: Write the MediaBlock**

Create `packages/landing-blocks/src/blocks/MediaBlock.svelte`:
- Props: `props: MediaBlockProps`
- `layoutClass`: full → 'w-full', contained → 'mx-auto max-w-4xl', split → 'mx-auto max-w-2xl'
- Image: `<img src={props.media.url} alt={props.media.alt} class="w-full rounded-[var(--radius-card)] object-cover" loading="lazy">`
- Caption: `<p class="mt-3 text-center text-sm text-[var(--color-muted)]">{props.caption}</p>`
- Lightbox: if `props.lightbox`, wrap in `<button onclick={() => lightboxOpen = true}>` + render a Bits UI `<Dialog.Root>` with full-size image when open
- State: `lightboxOpen` = false
- Container: `<section class="px-6 py-8"><div class="{layoutClass}">`

- [ ] **Step 2: Commit**

```bash
git add packages/landing-blocks/src/blocks/MediaBlock.svelte
git commit -m "feat(landing-blocks): add MediaBlock — single image with caption + lightbox"
```

---

## Task 17: Update Block Registry

**Files:**
- Modify: `packages/landing-blocks/src/render/blockRegistry.ts`

- [ ] **Step 1: Register all 15 new blocks**

Add imports for all 15 new block components and add entries to `blockRegistry`:

```typescript
import VideoBlock from '../blocks/VideoBlock.svelte';
import IconBlock from '../blocks/IconBlock.svelte';
import ContentBlock from '../blocks/ContentBlock.svelte';
import QuoteBlock from '../blocks/QuoteBlock.svelte';
import SpacerDividerBlock from '../blocks/SpacerDividerBlock.svelte';
import CountdownBlock from '../blocks/CountdownBlock.svelte';
import ComingSoonBlock from '../blocks/ComingSoonBlock.svelte';
import FormEmbedBlock from '../blocks/FormEmbedBlock.svelte';
import TimelineBlock from '../blocks/TimelineBlock.svelte';
import MapBlock from '../blocks/MapBlock.svelte';
import TableBlock from '../blocks/TableBlock.svelte';
import AdvertisementBlock from '../blocks/AdvertisementBlock.svelte';
import LiveRateTickerBlock from '../blocks/LiveRateTickerBlock.svelte';
import TabsBlock from '../blocks/TabsBlock.svelte';
import MediaBlock from '../blocks/MediaBlock.svelte';

// Add to blockRegistry:
video: { component: VideoBlock, icon: 'Video', label: 'Video', category: 'media' },
icon: { component: IconBlock, icon: 'Icon', label: 'Icon', category: 'media' },
content: { component: ContentBlock, icon: 'FileText', label: 'Rich Text', category: 'content' },
quote: { component: QuoteBlock, icon: 'Quote', label: 'Quote', category: 'content' },
spacer_divider: { component: SpacerDividerBlock, icon: 'SeparatorHorizontal', label: 'Spacer / Divider', category: 'layout' },
countdown: { component: CountdownBlock, icon: 'Timer', label: 'Countdown', category: 'utility' },
coming_soon: { component: ComingSoonBlock, icon: 'Clock', label: 'Coming Soon', category: 'utility' },
form_embed: { component: FormEmbedBlock, icon: 'FormInput', label: 'Form Embed', category: 'action' },
timeline: { component: TimelineBlock, icon: 'GitBranch', label: 'Timeline', category: 'content' },
map: { component: MapBlock, icon: 'Map', label: 'Map', category: 'geo' },
table: { component: TableBlock, icon: 'Table', label: 'Table', category: 'content' },
advertisement: { component: AdvertisementBlock, icon: 'Megaphone', label: 'Advertisement', category: 'ads' },
live_rate_ticker: { component: LiveRateTickerBlock, icon: 'TrendingUp', label: 'Live Rate Ticker', category: 'data' },
tabs: { component: TabsBlock, icon: 'PanelTop', label: 'Tabs', category: 'layout' },
media: { component: MediaBlock, icon: 'Image', label: 'Single Media', category: 'media' },
```

- [ ] **Step 2: Verify types compile**

Run: `cd /home/jmobile/websites/japa/japaconnect/apps/main && bunx svelte-check --tsconfig ./tsconfig.json 2>&1 | tail -5`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add packages/landing-blocks/src/render/blockRegistry.ts
git commit -m "feat(landing-blocks): register all 15 new blocks in blockRegistry"
```

---

## Task 18: Update Homepage Palette

**Files:**
- Modify: `packages/landing-blocks/src/palettes/landingPalettes.ts`

- [ ] **Step 1: Expand allowedBlocks**

Update `landingPalettes.homepage.allowedBlocks` to include all 23 block types:

```typescript
allowedBlocks: [
  'hero', 'carousel', 'card_grid', 'media_grid', 'entity_collection',
  'entity_feature', 'accordion', 'cta_banner', 'video', 'icon', 'content',
  'quote', 'spacer_divider', 'countdown', 'coming_soon', 'form_embed',
  'timeline', 'map', 'table', 'advertisement', 'live_rate_ticker',
  'tabs', 'media',
],
```

- [ ] **Step 2: Update defaultSeed**

Insert new sections into the default seed (Timeline after Featured Jobs, Map after Timeline, LiveRateTicker after Map, Video after CTA, Content before SpacerDivider). The full ordered seed:

1. Hero
2. MediaGrid (logo cloud)
3. CardGrid (features)
4. Carousel (tools)
5. EntityCollection (destinations)
6. EntityCollection (trending)
7. EntityFeature (university)
8. EntityCollection (blog)
9. EntityCollection (jobs)
10. Timeline (japa journey: Plan → Apply → Move → Settle)
11. Map (diaspora pins)
12. LiveRateTicker (remittance rates)
13. CardGrid (testimonials)
14. CardGrid (stats)
15. Countdown (optional — can be removed if no event)
16. Accordion (FAQ)
17. FormEmbed (newsletter)
18. CtaBanner (join)
19. Video (promo video)
20. Content (about text)
21. SpacerDivider

Add the new seed entries with geo-aware copy and sensible defaults. For Timeline, seed the 4 japa journey steps. For Map, seed with `geo_source: 'trending_spaces'`. For LiveRateTicker, seed with `geo_aware: true, layout: 'horizontal'`.

- [ ] **Step 3: Commit**

```bash
git add packages/landing-blocks/src/palettes/landingPalettes.ts
git commit -m "feat(landing-blocks): expand homepage palette to 23 blocks + updated default seed"
```

---

## Task 19: Integration Verification

- [ ] **Step 1: Run all unit tests**

Run: `cd /home/jmobile/websites/japa/japaconnect && bunx vitest run packages/landing-blocks/ 2>&1 | tail -20`
Expected: all tests pass

- [ ] **Step 2: Run svelte-check**

Run: `cd /home/jmobile/websites/japa/japaconnect/apps/main && bunx svelte-check --tsconfig ./tsconfig.json 2>&1 | tail -20`
Expected: no errors

- [ ] **Step 3: Verify all 23 blocks are registered**

Run: `cd /home/jmobile/websites/japa/japaconnect && node -e "const r = require('./packages/landing-blocks/src/render/blockRegistry.ts'); console.log(Object.keys(r.blockRegistry).length)"`
Expected: 23 (or verify via a quick grep count)

- [ ] **Step 4: Commit any fixes**

```bash
git add -A
git commit -m "fix: Plan 2 integration verification fixes"
```

---

## Self-Review Notes

**Spec coverage:** All 23 blocks from spec §3 are now implemented. §3 content/layout primitives (Hero, Carousel, Tabs, Accordion, CardGrid, MediaGrid, Media, Video, Icon, Content, Quote, CtaBanner, SpacerDivider) → Tasks 2-6, 15-16. §3 entity primitives (EntityFeature, EntityCollection) → Plan 1. §3 utility/special (Countdown, ComingSoon, FormEmbed, Timeline, Map, Table, Advertisement, LiveRateTicker) → Tasks 7-14. Tool registry → Plan 1 Task 15. §9 remittance rates → Task 14.

**Type consistency:** 15 new prop interfaces added in Task 1, used consistently in Tasks 2-16. `RemittanceRate` type from Plan 1 Task 15 used in Task 14. `Block` union expanded in Task 1, `blockRegistry` updated in Task 17 to match all 23 types.

**No placeholders:** All tasks have concrete file paths and implementation descriptions with specific prop fields, CSS classes, and component structure.

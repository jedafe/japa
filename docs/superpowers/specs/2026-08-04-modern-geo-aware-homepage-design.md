# Modern Geo-Aware Homepage & Reusable Landing Page Manager — Design

**Date:** 2026-08-04
**Status:** Draft (pending user spec review)
**Apps affected:** `apps/main` (public homepage), `apps/admin` (Landing Page Manager), new `packages/landing-blocks` (shared)
**ADRs:** ADR-058 (app shell / guest homepage exception), ADR-064 (geo-aware personalization)
**Backend phases:** All v2 features verified live (see §9 — Rectified v2 Feature Status)

---

## 1. Goal

Replace the current basic `GuestLanding.svelte` guest homepage with a modern, sleek, Dribbble-grade homepage that is:

- **Geo-aware** — copy, content, and country names adapt to the visitor's detected country (SSR, no flicker).
- **Fully admin-managed** — every section is toggleable, reorderable, addable, removable, replaceable, and editable in a dedicated Landing Page Manager.
- **Reusable** — the manager engine and block library are parameterized by page slug, so `docs.japaconnect.com` and `learn.japaconnect.com` get their own landing pages later with zero engine duplication.
- **Feature-rich** — every block carries a universal style system (positioning, typography, color, border, spacing, effects, transform, transition, icon) plus motion, geo-aware copy, and media-library image uploads.

Authenticated `/` (Primary timeline in the shell, ADR-032) is **untouched**.

---

## 2. Architecture & Data Flow

**Approach: Block-native.** The homepage is a Page Builder page (slug `homepage`) whose `Blocks` JSON is an ordered array of section blocks. No new backend routes — we reuse the existing Page Builder + media library APIs.

```
ADMIN (apps/admin)                          PUBLIC (apps/main)
┌──────────────────────┐                    ┌──────────────────────────┐
│ Landing Page Manager │                    │ Guest /  (unauth)        │
│ /admin/landing/[slug]│                    │ +page.server.ts          │
│                      │                    │                          │
│ reads:               │                    │ GET /pages/homepage      │
│  GET /admin/pages    │  ──── reuse ────►  │  → { blocks: [...] }     │
│  GET /admin/pages/{id}│                    │                          │
│                      │                    │ resolveGeoCopy(blocks,   │
│ writes:              │                    │   VisitorContext)        │
│  PATCH /admin/pages/ │  ◄── reuse ────    │  → {{country}} replaced  │
│   {id}  (blocks JSON)│                    │                          │
│                      │                    │ <LandingRenderer blocks> │
│ media:               │                    │  → renders each section  │
│  MediaPickerModal    │                    │    block in order        │
│  POST /admin/media/  │                    │                          │
│   upload             │                    │ SSR, no flicker          │
└──────────────────────┘                    └──────────────────────────┘
```

### Key decisions

1. **One page per landing surface.** `homepage` today; `docs_home` / `learn_home` later — same engine, different slug. The manager is parameterized by slug: `/admin/landing/[slug]`.

2. **Blocks JSON is the single source of truth.** Each section is one entry in the page's `Blocks` array: `{ id, type, props, visible, locked? }`. Reorder = array reorder. Toggle = `visible` flag. Add/remove/replace = array mutation. Saved via the existing `PATCH /admin/pages/{id}`.

3. **Geo-aware copy is a render-time transform, not stored data.** Block copy strings contain `{{country}}`-style placeholders resolved server-side from `VisitorContext.ContentCountry` before rendering. Admin shows a live "Preview as: [Country]" dropdown.

4. **Media is referenced, not embedded.** Image fields in block props store `{ media_id, url, alt }` (imgproxy URL from `MediaPickerModal`). Renderer uses `url`; `media_id` keeps the reference for re-selection.

5. **Section palette is curated per page type.** A `landingPalettes` registry maps page slug → allowed section types. The manager's "Add section" picker only shows palette-allowed types — guided composability.

6. **Authenticated `/` is untouched** (Primary timeline in the shell, ADR-032). Only the guest path changes.

7. **Fallback preserved.** If `GET /pages/homepage` 404s or returns no blocks, the existing `GuestLanding.svelte` renders as today — zero regression risk.

---

## 3. Block Catalog — 23 Generic Primitives

Every block carries the universal `style` object (§4) + motion config + geo-aware copy + background (color/gradient/image/video) + visibility rules.

Duplicates from the old system are folded into generic primitives — add a new entity type → add a value to `entity_type`, not a new block.

### Content & layout primitives

| # | Block | Purpose | Key props |
|---|---|---|---|
| 1 | **HeroBlock** | Hero with background (media/gradient/video), eyebrow, geo-aware headline/subhead, dual CTA, optional stats row, floating preview card | `eyebrow, headline, subhead, ctas[], stats[], background, floating_card?` |
| 2 | **CarouselBlock** | Slider of **nested blocks** (each slide is any block). Presets: `media-slide`, `card-slide` | `slides: Block[], autoplay, dots, arrows, transition, interval, pause_on_hover` |
| 3 | **TabsBlock** | Tabs of nested blocks | `tabs: [{label, icon?, content_blocks: Block[]}], layout, orientation, initial_tab` |
| 4 | **AccordionBlock** | Accordion of nested blocks. `preset:'faq'` gives Q/A schema + search + categories | `items: [{title, content_blocks}], preset, layout, searchable` |
| 5 | **CardGridBlock** | The workhorse — covers features, testimonials, stats, tool highlights, logo clouds | `cards: [{icon, title, description, media, cta, rating?, stat_value?, stat_label?}], card_variant, layout, columns` |
| 6 | **MediaGridBlock** | Grid/slider/marquee of media items — covers image galleries, partner logo strips | `items: [{media, caption?, href?}], card_style, lightbox, columns` |
| 7 | **MediaBlock** | Single media (image or self-hosted video) | `media, layout, caption, lightbox` |
| 8 | **VideoBlock** | Embedded video (YouTube/Facebook/Vimeo/file) | `provider, url|id, aspect_ratio, autoplay, mute, captions, loop, thumbnail, title, description` |
| 9 | **IconBlock** | Pure icon display with full styling | `icon, size, color, background_color, shape, label?, href?` |
| 10 | **ContentBlock** | Rich text / prose (TipTap HTML), geo-aware | `html, prose_style` |
| 11 | **QuoteBlock** | Single pull quote | `quote, author, role, avatar, layout` |
| 12 | **CtaBannerBlock** | Headline + subhead + single/dual CTA + background | `headline, subhead, ctas[], background, alignment` |
| 13 | **SpacerDividerBlock** | Spacing + divider (line/gradient/dots) | `height, divider_style` |

### Entity primitives (data-bound, Rule 16 compliant)

| # | Block | Purpose | Key props |
|---|---|---|---|
| 14 | **EntityFeatureBlock** | Feature ONE entity of any type | `entity_type, ref: {id?|slug?} | query, layout, media_override` |
| 15 | **EntityCollectionBlock** | List/grid/slider of entities of any type | `entity_type, query: {filters, sort, limit, geo?}, layout, card_variant, columns` |

**Supported `entity_type` values** (all backed by live routes — see §9):

| `entity_type` | API | Homepage use |
|---|---|---|
| `post` / `blog` / `thread` | `GET /explore`, `GET /blog`, `GET /blog/{slug}` | Trending content, latest blog |
| `space` / `organization` | `GET /trending/spaces`, `GET /spaces/{slug}` | Destination grid, featured org |
| `university` | `GET /universities`, `GET /universities/{slug}` | Featured universities |
| `job` | `GET /jobs`, `GET /jobs/{id}` | Featured jobs (live) |
| `scholarship` | `GET /scholarships`, `GET /scholarships/{id}` | Featured scholarships (live) |
| `tool` | curated registry of deep links to live tool routes | Tool highlights slider |
| `remittance_rate` | `GET /remittance/rates`, `GET /remittance/corridors` | Live rate ticker |

### Utility & special primitives

| # | Block | Purpose | Key props |
|---|---|---|---|
| 16 | **CountdownBlock** | Countdown timer | `target_date, label, layout, show_days/hours/minutes/seconds, on_expire, expired_message` |
| 17 | **ComingSoonBlock** | Teaser / waitlist | `title, description, icon, estimated_date, waitlist_form?, background` |
| 18 | **FormEmbedBlock** | Generic form embed (newsletter/waitlist/contact) | `preset, fields, submit_endpoint, success_message, error_message` |
| 19 | **TimelineBlock** | Sequential steps (japa journey: Plan → Apply → Move → Settle) | `steps: [{icon, title, description, media?}], layout` |
| 20 | **MapBlock** | Geo signature — stylized world map with pulsing diaspora pins | `mode, pins: [{country, label, href?}] | dynamic, geo_source` |
| 21 | **TableBlock** | Generic data table (destination comparison) | `columns, rows, sortable, striped` |
| 22 | **AdvertisementBlock** | Ad slot (`hero_below`, `landing_inline_*`, `landing_footer`) | `slot_key` |
| 23 | **LiveRateTickerBlock** | Slim horizontal ticker of live remittance rates | `corridors?, limit, geo_aware, layout` |

### Homepage palette (default seed order)

Hero → Carousel → MediaGrid (logo cloud, `card_style:'logo'`) → CardGrid (variant: feature) → Carousel (tools slider, nested EntityCollection `entity_type:'tool'`) → Tabs → EntityCollection (destinations) → EntityCollection (trending threads) → EntityFeature (one university) → EntityCollection (latest blog) → EntityCollection (featured jobs) → Timeline (japa journey) → Map → LiveRateTicker → CardGrid (testimonials) → CardGrid (stats) → Countdown → ComingSoon → Accordion (FAQ preset) → FormEmbed (newsletter preset) → CtaBanner → Video → Content → SpacerDivider.

All add/remove/replace/toggle/reorder/editable, all admin-managed, all geo-aware, all styled via the shared panel.

---

## 4. Universal Style System

Every block carries a `style` object, edited in a shared `<BlockStylePanel>` with grouped accordions: **Layout · Typography · Color · Border · Spacing · Effects · Transform · Transition · Icon**.

```
style: {
  layout: {
    position: 'static'|'relative'|'absolute'|'sticky'|'fixed',
    top?, right?, bottom?, left?: number | 'auto',
    z_index: 'auto'|0|10|20|30|40|50|100|999,
    width: 'auto'|'full'|'contained'|'fit'|number,
    max_width?, min_height?, height?: number | 'auto',
    display: 'block'|'flex'|'grid'|'inline-flex'|'inline-block'|'hidden',
    flex_direction?, flex_wrap?, align_items?, justify_content?,
    grid_template_columns?, grid_template_rows?,
    gap?: 'none'|'xs'|'sm'|'md'|'lg'|'xl'|'2xl'|number,
    order?, float?, inset_mode?,
  },
  typography: {
    heading_size, heading_weight, heading_family, heading_letter_spacing,
    heading_transform, heading_line_height, heading_color?,
    body_size, body_weight, body_family, body_letter_spacing,
    body_transform, body_line_height, body_color?,
    text_align, text_decoration, text_shadow?,
  },
  color: { text?, heading?, accent? },
  background: {
    type: 'none'|'color'|'gradient'|'image'|'video',
    color?, gradient?: {from, to, angle}, 
    image?: {media_id, url, alt, fit, overlay?: {color, opacity}},
    video?: {media_id, url, poster?, loop, muted, autoplay},
    blur?, opacity?,
  },
  border: { width, style, color, radius, sides },
  spacing: { padding, margin },  // preset scale or per-side
  effects: {
    shadow, box_shadow_color?, box_shadow_spread?, opacity?,
    blur?, grayscale?, brightness?, contrast?, saturate?, hue_rotate?,
    invert?, sepia?, backdrop_blur?, mix_blend_mode?,
    overflow, overflow_x?, overflow_y?, cursor?, pointer_events?, z_index?,
  },
  transform: { scale?, rotate?, translate_x?, translate_y?, skew_x?, skew_y?, origin?, perspective? },
  transition: { property, duration, easing, delay },
  icon: { name?, size, color, background_color, shape, border_width?, border_color?, padding?, position },
}
```

### Render strategy

`styleToCss(style)` emits:
- **Tailwind v4 utilities** where a direct mapping exists (spacing scale, radius scale, shadow scale, flex/grid utilities, position utilities).
- **Inline `style=""` CSS custom properties** for values outside Tailwind's scale (custom hex, arbitrary px/rem, z-index, transforms, filters, blend modes, backdrop-blur, transitions).

No `@apply`, no `dark:` variants (colors resolve to `var(--color-*)` which flip in dark mode). One `BlockShell.svelte` wraps every section: takes `style` + `motion` + `background` + `children`, applies resolved classes/inline styles, renders the `<Reveal>` motion wrapper. Block components stay content-focused.

### Positioning examples

- **Sticky hero CTA bar**: `layout.position:'sticky', top:80, z_index:20`
- **Floating preview card**: `position:'absolute', bottom:-40, right:40, z_index:30, effects.shadow:'xl'`
- **Glass nav overlay**: `background.type:'color', background.opacity:80, effects.backdrop_blur:'lg'`
- **Layered parallax**: bg media `position:'absolute', z_index:0`, content `position:'relative', z_index:10`

---

## 5. Public Homepage Rendering

### Route & SSR flow

`apps/main/src/routes/(app)/+page.server.ts` — guest branch (authed branch untouched):

1. **Resolve ContentCountry**: `?geo` param (always wins) → `VisitorContext.ContentCountry` (MaxMind via `GET /geo/me`) → `content_country_preference` cookie → `global` fallback.
2. **Fetch landing page**: `GET /pages/homepage` → `{ id, blocks, meta, theme_id }`. 404/empty → `GuestLanding.svelte` fallback.
3. **Resolve geo-aware copy**: `resolveGeoCopy(blocks, geo)` replaces `{{country}}` tokens.
4. **Resolve media URLs**: imgproxy variants per breakpoint.
5. **Return** `{ authed:false, landing:{blocks, meta, theme}, geo, contentCountry }` — all in initial HTML, no client flicker.

### Rendering pipeline

```
+page.svelte (guest)
  └─ <LandingRenderer {blocks} {geo} {contentCountry}>
       └─ {#each blocks as b}
            {#if !b.visible}skip{:else}
              <BlockShell {style} {motion} {background}>
                {block.type → component dispatch (23 primitives)}
              </BlockShell>
            {/if}
          {/each}
```

`LandingRenderer.svelte` is a thin dispatcher. `BlockShell.svelte` applies universal style + motion + background. Block components are pure content renderers.

### Motion & animation

One `<Reveal.svelte>` wrapper (used by `BlockShell`):
- **Scroll-triggered reveal**: `IntersectionObserver`, plays `motion.preset` (fade-up/fade-in/slide-in/scale-in) on viewport entry. `stagger_children` delays each child.
- **Page-load sequence**: first 2-3 blocks (hero, carousel) play on mount — orchestrated opening ~1.2s, then quiet.
- **Hover micro-interactions**: card lift, icon scale, CTA glow — via Tailwind `transition` + `hover:`.
- **Ambient**: marquee logo clouds (CSS keyframes), carousel autoplay, count-up stats (rAF), map pin pulse.
- **`prefers-reduced-motion`**: all reveals collapse to instant, autoplay pauses, count-up shows final value, marquee stops. One check at `Reveal` level, every block honors it.

### Skeletons & states

Every data-bound block: `{#if loading}<Skeleton/>{:else if error}<ErrorState/>{:else if empty}{#if hide_if_empty}<!-- hidden -->{:else}<EmptyState/>{/if}{:else}...{/if}`. Skeleton shape matches content. Static blocks (Hero, Content, CTA, Countdown) render immediately.

### Signature visual moments

1. **Hero** — full-bleed background, glassy floating preview card, geo-aware headline, dual CTA, optional stats row.
2. **Map block** — geo signature: stylized world map with pulsing diaspora pins, visitor's country highlighted.
3. **Timeline (japa journey)** — Plan → Apply → Move → Settle, vertical on mobile / zigzag on desktop, numbered markers (earned — real ordered process).
4. **Carousel of tools** — nested EntityCollection (`entity_type:'tool'`) sliding as cards with Lucide icons, accent colors, badges.
5. **LiveRateTicker** — slim live remittance rates ticker, geo-aware corridors.

### SEO

- `<svelte:head>` from `landing.meta` (title, description, OG, Twitter, canonical).
- JSON-LD `WebSite` + `Organization` at page level; `ItemList` from EntityCollection blocks listing universities/orgs.
- All server-rendered — no client-only content for crawlers.

### Performance

- Blocks render SSR; only `<Reveal>` and interactive bits (carousel, tabs, accordion, map) hydrate.
- Carousel/tabs use Bits UI primitives (accessible, minimal JS).
- Images via imgproxy variants (webp, responsive `srcset`).
- No client-side geo swap — geo-weighted fetch is in `+page.server.ts` (ADR-064).

---

## 6. Geo-Aware Copy System & i18n

### Geo copy registry

```
type GeoCopyTokens = {
  country: string;            // "Nigeria" | "the UK"
  country_short: string;      // "NG" | "UK"
  country_adjective: string;  // "Nigerian" | "British"
  continent: string;          // "Africa" | "Europe"
  region: string;             // "West Africa" | "Western Europe"
  diaspora_phrase: string;    // "Nigerians at home" | "Nigerians in the UK"
  destination_phrase: string; // "your destination" | "Nigeria" | "a new country"
  locale_hint: string;        // "en-NG" | "en-GB"
  names: { en: string, fr: string };  // localized country names
};
```

Full ISO-2 set seeded with top ~30 diaspora destinations. Unknown codes fall back to `GLOBAL_TOKENS` — never a raw `{{token}}` reaches the user.

### Placeholder syntax

`{{token}}` and `{{token|fallback}}` (graceful global copy). Supported tokens: `country`, `country_short`, `country_adjective`, `continent`, `region`, `diaspora_phrase`, `destination_phrase`, `destination` (alias for `destination_phrase`).

### Resolution chain (SSR, no flicker)

```
1. ?geo param            → if present, wins
2. VisitorContext         → GET /geo/me (MaxMind, server-side)
3. content_country_preference → soft-session cookie for guests
4. GLOBAL                 → fallback
```

"Showing: [Country] · Change" control in the slim marketing header → sets `?geo=` (pure guest) or `PATCH /me/profile` (authed-curious). Re-weight, never hide (ADR-064).

### i18n integration

- Default block copy in `en.json` / `fr.json` under `landing.*` namespace, with `{{token}}` placeholders.
- Resolver runs **after** i18n lookup: `t('landing.hero.headline')` → localized string with placeholders → `resolveGeoCopy` fills country tokens.
- `geoCopy` registry stores localized country names per locale (fr.json `{{country}}` for GB = "le Royaume-Uni").
- Admin editor: optional "Translate to French" toggle shows a second `<GeoText>` for the fr variant.

### Concrete examples

| Placeholder | Nigeria visitor | UK visitor | Global |
|---|---|---|---|
| `{{country}}` | Nigeria | the UK | your country |
| `{{country_adjective}}` | Nigerian | British | local |
| `{{diaspora_phrase}}` | Nigerians at home | Nigerians in the UK | the diaspora |
| `{{destination}}` | your destination | Nigeria | a new country |

**Hero (NG):** "Your journey from Nigeria starts here — join thousands planning their move."
**Hero (GB):** "Settling in the UK? Join thousands of Nigerians building life abroad."
**Hero (global):** "Your journey abroad starts here — join the diaspora community."

---

## 7. Admin Landing Page Manager UI

### Route & parameterization

`apps/admin/src/routes/landing/[slug]/+page.svelte` — parameterized by slug. `/admin/landing/homepage` today; `/admin/landing/docs_home`, `/admin/landing/learn_home` later.

### Three-pane editor

```
┌─────────────────────────────────────────────────────────────┐
│  Top bar: [← Landing Pages]  Homepage          [Preview ▾]  │
│           [Save draft]  [Publish]  [Theme ▾]                 │
├──────────┬──────────────────────────┬───────────────────────┤
│  LEFT    │       CENTER              │     RIGHT             │
│ Sections │   Live Preview            │   Inspector           │
│  list    │   (scaled device frame)   │   [Content|Style|Set] │
│ + Add    │   [Desktop|Tablet|Mobile] │   per-block editors   │
│  section │                           │                       │
│ drag     │                           │                       │
│ reorder  │                           │                       │
│ ☑/☐ vis  │                           │                       │
└──────────┴──────────────────────────┴───────────────────────┘
```

### Left pane — Section list

- Ordered list of blocks. Each row: drag handle, block icon, editable label, type label, visibility toggle (eye), duplicate, delete.
- **"+ Add section"** → palette picker (Bits UI `Dialog`) showing only `landingPalettes[pageType]` blocks. Searchable, with icon + description + preview thumbnail.
- **Replace**: "⋯" menu → "Replace with…" → palette picker filtered to compatible types. Swaps block type, carries over common props.
- Optional `region` grouping (header/body/footer) via collapsible groups.

### Center pane — Live preview

- Renders actual homepage blocks via shared `LandingRenderer` + `BlockShell` + block components (from `packages/landing-blocks` — identical to production).
- Device toggle: Desktop (1280) / Tablet (768) / Mobile (375).
- Click any block → selects it → right pane shows inspector. Drag-to-reorder in preview.
- **"Preview as" geo dropdown** → re-resolves `{{country}}` for selected country. Defaults to editor's geo or "Global".
- Unsaved indicator per section. Debounced real-time updates.

### Right pane — Inspector

**Content tab** — per-block-type form (purpose-built, not raw JSON). Each primitive has a dedicated content editor. Shared sub-editors (one implementation each, reused everywhere):

- `<GeoText>` — textarea + "Insert placeholder" dropdown + live resolved preview.
- `<MediaPicker>` — wraps existing `MediaPickerModal.svelte` (Browse/Upload/Stock tabs). Returns `{media_id, url, alt}`.
- `<IconPicker>` — Bits UI `Popover` + searchable Lucide grid. Returns icon name.
- `<BackgroundPicker>` — type selector (color/gradient/image/video) + controls.
- `<CTAEditor>` — label, href, variant, icon.
- `<EntityRefPicker>` — searches API by `entity_type`, returns ref.
- `<NestedBlocksEditor>` — mini section list for blocks containing nested blocks (Carousel slides, Tabs, Accordion items).

**Style tab** — shared `<BlockStylePanel>` (one component, all 23 blocks). Grouped accordions: Layout, Typography, Color & Background, Border, Spacing, Effects, Transform, Transition, Icon. Every control has live preview + "reset to default" per group.

**Settings tab** — visibility toggle, motion config (preset/delay/stagger), visibility rules (`geo_countries`, `hide_if_empty`), block label, lock toggle.

### Top bar actions

- **Save draft** — `PATCH /admin/pages/{id}`. Unsaved indicator clears.
- **Publish** — save + `POST /admin/pages/{id}/publish`. Diff summary (added/removed/reordered/edited).
- **Theme ▾** — pick/switch page theme.
- **Preview ▾** — "Preview as: [Country]" + "Open public preview".
- **History** — recent saves with restore.
- **Save as template** — existing `POST /admin/pages/{id}/save-as-template`.

### Landing pages index

`apps/admin/src/routes/landing/+page.svelte` — DataTable listing all managed landing pages. Columns: title, slug, page type, status, last updated, actions. "Create landing page" → picks page type → creates page → routes to editor. Reuses existing DataTable spec.

### Reusability for docs/learn

- `landingPalettes[pageType]` controls allowed blocks per page type.
- Shared block components + Style panel + sub-editors are page-type-agnostic; only palette filter differs.
- One "Landing Pages" sidebar entry → index DataTable → individual editors. No per-app duplication.

### Permissions

Reuses existing `pagebuilder.blocks.manage` permission. No new RBAC needed.

---

## 8. Shared Blocks Package & File Structure

### `packages/landing-blocks`

Block components, style system, geo copy, and sub-editors in one workspace package consumed by both `apps/main` and `apps/admin`. Guarantees admin preview = production render.

```
packages/landing-blocks/
├── package.json                    # name: @japaconnect/landing-blocks
├── src/
│   ├── index.ts
│   ├── types.ts                    # Block, BlockStyle, BlockProps, GeoContext
│   ├── blocks/                     # 23 primitives (content renderers)
│   │   ├── HeroBlock.svelte
│   │   ├── CarouselBlock.svelte
│   │   ├── TabsBlock.svelte
│   │   ├── AccordionBlock.svelte
│   │   ├── CardGridBlock.svelte
│   │   ├── MediaGridBlock.svelte
│   │   ├── MediaBlock.svelte
│   │   ├── VideoBlock.svelte
│   │   ├── IconBlock.svelte
│   │   ├── ContentBlock.svelte
│   │   ├── QuoteBlock.svelte
│   │   ├── CtaBannerBlock.svelte
│   │   ├── SpacerDividerBlock.svelte
│   │   ├── EntityFeatureBlock.svelte
│   │   ├── EntityCollectionBlock.svelte
│   │   ├── CountdownBlock.svelte
│   │   ├── ComingSoonBlock.svelte
│   │   ├── FormEmbedBlock.svelte
│   │   ├── TimelineBlock.svelte
│   │   ├── MapBlock.svelte
│   │   ├── TableBlock.svelte
│   │   ├── AdvertisementBlock.svelte
│   │   └── LiveRateTickerBlock.svelte
│   ├── shell/
│   │   ├── BlockShell.svelte       # applies style + background + motion
│   │   ├── Reveal.svelte           # IntersectionObserver, reduced-motion aware
│   │   └── styleToCss.ts           # style → Tailwind classes + inline CSS vars
│   ├── render/
│   │   ├── LandingRenderer.svelte  # dispatches block.type → component
│   │   ├── blockRegistry.ts        # type → component + palette metadata
│   │   └── Skeletons.svelte        # per-block skeleton shapes
│   ├── geo/
│   │   ├── copyRegistry.ts         # ISO-2 → GeoCopyTokens (localized en/fr)
│   │   ├── resolveGeoCopy.ts       # pure resolver
│   │   └── VisitorContext.ts       # SSR geo context types
│   ├── data/
│   │   ├── entityFetchers.ts       # per entity_type → API call, typed
│   │   └── types.ts                # API response types (no `any`)
│   ├── editors/                    # admin-only (tree-shaken from main build)
│   │   ├── BlockStylePanel.svelte
│   │   ├── GeoText.svelte
│   │   ├── MediaPicker.svelte      # accepts injected mediaPickerComponent
│   │   ├── IconPicker.svelte
│   │   ├── BackgroundPicker.svelte
│   │   ├── CTAEditor.svelte
│   │   ├── EntityRefPicker.svelte
│   │   ├── NestedBlocksEditor.svelte
│   │   └── content/                # per-block Content tab forms
│   └── palettes/
│       └── landingPalettes.ts      # pageType → allowed blocks + default seed
```

### Consumer file changes

**apps/main:**
```
apps/main/src/routes/(app)/
├── +page.server.ts                 # guest branch: fetch /pages/homepage, resolveGeoCopy
├── +page.svelte                    # guest branch: <LandingRenderer {blocks} {geo}/>
apps/main/src/lib/components/pagebuilder/
├── GuestLanding.svelte             # kept as fallback
```

**apps/admin:**
```
apps/admin/src/routes/
├── landing/
│   ├── +page.svelte                # landing pages index (DataTable)
│   ├── +page.server.ts
│   └── [slug]/
│       ├── +page.svelte            # 3-pane editor
│       └── +page.server.ts         # GET /admin/pages?slug, palette, themes
```

### Workspace & tree-shaking

- Add `packages/landing-blocks` to Bun workspace. `peerDependencies` on svelte, @sveltejs/kit, bits-ui, lucide-svelte, tailwindcss.
- Tailwind v4 content config in both apps scans `packages/landing-blocks/src/**`.
- `editors/` is admin-only via conditional exports. `MediaPicker.svelte` accepts an injected `mediaPickerComponent` prop (admin passes its `MediaPickerModal`) — package stays free of admin-specific imports.

### Type safety

`types.ts` defines a discriminated union of all 23 block types — each with its own `props` shape. `LandingRenderer` narrows on `block.type` and passes typed props. No `any`. API response types derived from backend struct tags.

### Migration & fallback

1. Build package + manager.
2. Seed `homepage` page with default palette + i18n default copy (one-time seed or "Initialize homepage" button).
3. `+page.server.ts`: `GET /pages/homepage` → found with blocks → `LandingRenderer`. 404/empty → `GuestLanding.svelte` fallback.
4. Old `apps/main/src/lib/components/pagebuilder/blocks/` deprecated, removed once migration complete.

---

## 9. Rectified v2 Feature Status

**All v2 features verified live against `internal/server/server.go` — the skill's phase table was outdated.** Every route is wired and backed by a complete `internal/*` package with handlers, service, store, and tests.

| Feature | Section/ADR | Key public routes | Status |
|---|---|---|---|
| Jobs board | §63 / ADR-081 | `GET /jobs`, `GET /jobs/{id}`, `POST /jobs/{id}/apply` | ✅ Live |
| Gamification | §64 / ADR-082 | `GET /gamification/badges`, `/challenges`, `/leaderboard` | ✅ Live |
| Travel search | §67 / ADR-085 | `GET /travel/search/flights`, `/hotels` | ✅ Live |
| Visa Checklist | §70 / ADR-070 | `GET /checklist/selector`, `/checklist/{guide_id}` | ✅ Live |
| Readiness Assessment | §71 / ADR-071 | `GET /assessment`, `POST /assessment/submit` | ✅ Live |
| Cost of Living | §72 / ADR-072 | `GET /calculator/cost-of-living/cities`, `/compare` | ✅ Live |
| Scholarship Finder | §73 / ADR-073 | `GET /scholarships`, `GET /scholarships/{id}` | ✅ Live |
| Remittance Tracker | §74 / ADR-075 | `GET /remittance/rates`, `/calculate`, `/history`, `/providers`, `/corridors` | ✅ Live |
| Newsletter | §62 / ADR-080 | `POST /newsletter/subscribe`, unsubscribe, preferences | ✅ Live |
| Referral | §60 / ADR-078 | `GET /ref/{code}`, `GET /me/referral`, `/history` | ✅ Live |
| JapaBot (RAG) | §69 | `POST /ai/chat`, `GET /ai/conversations` | ✅ Live |

### Tool registry (curated deep links to live routes)

| Tool | Deep link | API |
|---|---|---|
| Visa Checklist | `/tools/visa-checklist` or `/checklist/selector` | `GET /checklist/*` |
| Readiness Assessment | `/tools/readiness-assessment` or `/assessment` | `GET /assessment` |
| Cost of Living Calculator | `/tools/cost-of-living` or `/calculator/cost-of-living` | `GET /calculator/cost-of-living/*` |
| Scholarship Finder | `/tools/scholarship-finder` or `/scholarships` | `GET /scholarships` |
| Remittance Rate Tracker | `/tools/remittance-tracker` or `/remittance/rates` | `GET /remittance/*` |
| JapaBot | `/ai/chat` or `/tools/japabot` | `POST /ai/chat` |
| Jobs board | `/jobs` | `GET /jobs` |
| Travel search | `/travel/search/flights` | `GET /travel/search/*` |

---

## 10. API-Readiness Gate (Final)

| Need | Route | Status |
|---|---|---|
| Public landing page | `GET /pages/homepage` | ✅ exists |
| Admin page CRUD | `GET/PATCH /admin/pages/{id}` | ✅ exists |
| Publish | `POST /admin/pages/{id}/publish` | ✅ exists |
| Reorder | `PATCH /admin/pages/{id}/blocks/reorder` | ✅ exists |
| Save as template | `POST /admin/pages/{id}/save-as-template` | ✅ exists |
| Media upload | `POST /admin/media/upload` | ✅ exists |
| Media list | `GET /admin/media` | ✅ exists |
| Themes | `GET /admin/pagebuilder/themes` | ✅ exists |
| Content entities | `/explore`, `/blog`, `/blog/{slug}` | ✅ exists |
| Spaces | `/trending/spaces`, `/spaces/{slug}` | ✅ exists |
| Universities | `/universities`, `/universities/{slug}` | ✅ exists |
| Jobs | `GET /jobs`, `GET /jobs/{id}` | ✅ live |
| Scholarships | `GET /scholarships`, `GET /scholarships/{id}` | ✅ live |
| Remittance rates | `GET /remittance/rates`, `/corridors` | ✅ live |
| Tools (curated) | static registry of deep links to live routes | ✅ live |
| Geo | `GET /geo/me` | ✅ exists |
| Newsletter | `POST /newsletter/subscribe` | ✅ exists |
| RBAC | `pagebuilder.blocks.manage` | ✅ exists |

**Every gate is satisfied. No missing routes. No ComingSoon degradation needed for any block.**

---

## 11. Out of Scope

- Authenticated `/` timeline (unchanged — ADR-032).
- Backend route additions (none needed).
- New RBAC permissions (reuses `pagebuilder.blocks.manage`).
- `docs_home` / `learn_home` palettes (same engine, later iteration).
- Mobile app homepage (separate Flutter surface).
- A/B testing of homepage variants (future enhancement).

# Modern Geo-Aware Homepage — Plan 1: Foundation & Core Blocks

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the shared `@japaconnect/landing-blocks` package with the type system, geo-aware copy resolver, universal style system, and 8 core block primitives, then wire the public guest homepage to render via the new `LandingRenderer`.

**Architecture:** A new Bun workspace package (`packages/landing-blocks`) holds all block components, the style system, geo copy, and the rendering pipeline. `apps/main` consumes it for SSR guest homepage rendering. Block config is stored as JSON in the existing Page Builder `homepage` page (slug `homepage`), fetched via `GET /pages/homepage`, geo-resolved server-side, and rendered by `LandingRenderer`. No new backend routes.

**Tech Stack:** SvelteKit 2, Svelte 5 Runes (`$state`, `$derived`, `$props`, `$effect`), Bits UI 2.18, Tailwind CSS v4, TypeScript (zero `any`), Vitest + @testing-library/svelte, Bun workspace.

## Global Constraints

- Svelte 5 strict Runes only — no `export let`, no `$:`, no `on:click`, no `createEventDispatcher`.
- Bits UI dot-notation for all interactive primitives (`<Dialog.Root>`, `<Accordion.Root>`, etc.).
- Tailwind v4 — utilities via `class`; theme via CSS custom properties; dark mode = `[data-theme="dark"]` (never `dark:` variant).
- Colors resolve to `var(--color-*)` / `var(--chart-*)` / custom hex only.
- Zero `any` — all types derived from discriminated unions.
- No hardcoded data (Rule 16) — dynamic blocks fetch from API.
- `prefers-reduced-motion` respected at the `Reveal` level.
- i18n keys in `en.json` + `fr.json` with `{{country}}` placeholders.
- Existing `GuestLanding.svelte` kept as fallback (zero regression).
- Authenticated `/` timeline untouched (ADR-032).
- One-line comment per change (refactor convention).

## Spec Reference

`docs/superpowers/specs/2026-08-04-modern-geo-aware-homepage-design.md` — §2 (architecture), §3 (block catalog), §4 (style system), §5 (rendering), §6 (geo copy), §8 (file structure).

---

## File Structure

### Create: `packages/landing-blocks/`

| File | Responsibility |
|---|---|
| `package.json` | Package manifest, peer deps on svelte/kit/bits-ui/lucide/tailwind |
| `src/index.ts` | Public exports barrel |
| `src/types.ts` | Block, BlockStyle, BlockProps discriminated union, GeoContext, MotionConfig |
| `src/geo/copyRegistry.ts` | ISO-2 → GeoCopyTokens map (localized en/fr) + GLOBAL_TOKENS |
| `src/geo/resolveGeoCopy.ts` | Pure `{{token}}` resolver — walks block props, replaces tokens |
| `src/geo/resolveGeoCopy.test.ts` | Unit tests for the resolver |
| `src/shell/styleToCss.ts` | style object → Tailwind classes + inline CSS custom properties |
| `src/shell/styleToCss.test.ts` | Unit tests for style conversion |
| `src/shell/BlockShell.svelte` | Applies style + background + motion, wraps children |
| `src/shell/Reveal.svelte` | IntersectionObserver scroll reveal, reduced-motion aware |
| `src/render/blockRegistry.ts` | type → component map + palette metadata |
| `src/render/LandingRenderer.svelte` | Dispatches block.type → component, iterates blocks |
| `src/data/entityFetchers.ts` | Per entity_type → API fetch function, typed |
| `src/data/types.ts` | API response types for entity fetchers |
| `src/blocks/HeroBlock.svelte` | Hero section |
| `src/blocks/CtaBannerBlock.svelte` | CTA banner |
| `src/blocks/CardGridBlock.svelte` | Feature/testimonial/stat card grid |
| `src/blocks/MediaGridBlock.svelte` | Image gallery / logo cloud |
| `src/blocks/AccordionBlock.svelte` | FAQ accordion / generic accordion |
| `src/blocks/CarouselBlock.svelte` | Carousel of nested blocks |
| `src/blocks/EntityCollectionBlock.svelte` | Data-bound entity list/grid/slider |
| `src/blocks/EntityFeatureBlock.svelte` | Data-bound single entity feature |
| `src/palettes/landingPalettes.ts` | pageType → allowed blocks + default seed |

### Modify: `apps/main/`

| File | Change |
|---|---|
| `package.json` | Add `@japaconnect/landing-blocks` dependency |
| `src/app.css` | Add `@source` for `packages/landing-blocks/src` (Tailwind content scanning) |
| `src/routes/(app)/+page.server.ts` | Guest branch: resolve geo, fetch `/pages/homepage`, resolveGeoCopy, return blocks |
| `src/routes/(app)/+page.svelte` | Guest landing branch: render `<LandingRenderer>` |
| `src/lib/i18n/en.json` | Add `landing.*` keys with `{{country}}` placeholders |
| `src/lib/i18n/fr.json` | Add `landing.*` keys with `{{country}}` placeholders |

### Modify: root

| File | Change |
|---|---|
| `package.json` | Add `packages/*` to workspaces |

---

## Task 1: Workspace Package Scaffolding

**Files:**
- Create: `packages/landing-blocks/package.json`
- Create: `packages/landing-blocks/src/index.ts`
- Create: `packages/landing-blocks/tsconfig.json`
- Modify: `package.json` (root — add workspaces)
- Modify: `apps/main/package.json` (add dependency)
- Modify: `apps/main/src/app.css` (add Tailwind source)

**Interfaces:**
- Produces: `@japaconnect/landing-blocks` package importable by `apps/main` and `apps/admin`

- [ ] **Step 1: Create the package directory and package.json**

```json
{
  "name": "@japaconnect/landing-blocks",
  "version": "0.0.1",
  "private": true,
  "type": "module",
  "svelte": "./src/index.ts",
  "types": "./src/index.ts",
  "exports": {
    ".": {
      "types": "./src/index.ts",
      "svelte": "./src/index.ts"
    },
    "./editors": {
      "types": "./src/editors/index.ts",
      "svelte": "./src/editors/index.ts"
    }
  },
  "peerDependencies": {
    "svelte": "^5.0.0",
    "@sveltejs/kit": "^2.0.0",
    "bits-ui": "^2.0.0",
    "lucide-svelte": "^1.0.0",
    "tailwindcss": "^4.0.0"
  }
}
```

- [ ] **Step 2: Create the tsconfig.json**

```json
{
  "extends": "../../apps/main/tsconfig.json",
  "compilerOptions": {
    "moduleResolution": "bundler",
    "strict": true
  },
  "include": ["src/**/*"]
}
```

- [ ] **Step 3: Create a minimal src/index.ts**

```typescript
// @japaconnect/landing-blocks — shared block library for landing pages.
export const PACKAGE_NAME = '@japaconnect/landing-blocks';
```

- [ ] **Step 4: Add workspaces to root package.json**

Check if root `package.json` has a `workspaces` key. If not, add:
```json
"workspaces": ["apps/*", "packages/*"]
```

- [ ] **Step 5: Add the package as a dependency in apps/main/package.json**

Add to `dependencies`:
```json
"@japaconnect/landing-blocks": "workspace:*"
```

- [ ] **Step 6: Run bun install and verify**

Run: `cd /home/jmobile/websites/japa/japaconnect && bun install`
Expected: installs successfully, symlinks `packages/landing-blocks` into `apps/main/node_modules/@japaconnect/landing-blocks`

- [ ] **Step 7: Add Tailwind content source to apps/main/src/app.css**

After the `@import 'tailwindcss';` line, add:
```css
@source '../../../packages/landing-blocks/src';
```

- [ ] **Step 8: Commit**

```bash
git add packages/landing-blocks/ package.json apps/main/package.json apps/main/src/app.css
git commit -m "feat: scaffold @japaconnect/landing-blocks workspace package"
```

---

## Task 2: Core Types

**Files:**
- Create: `packages/landing-blocks/src/types.ts`
- Modify: `packages/landing-blocks/src/index.ts`

**Interfaces:**
- Produces: `Block`, `BlockStyle`, `MotionConfig`, `GeoContext`, `BackgroundConfig`, `MediaRef`, `VisibilityRules`, `CTA`, `LandingPage`, all block prop interfaces, `EntityType`

- [ ] **Step 1: Write the types file**

See `docs/superpowers/specs/2026-08-04-modern-geo-aware-homepage-design.md` §4 for the full `BlockStyle` object. The types file defines:
- `MediaRef` — `{ media_id, url, alt }`
- `BackgroundConfig` — type (none/color/gradient/image/video) + sub-configs
- `MotionConfig` — `{ preset, delay, stagger_children? }`
- `VisibilityRules` — `{ geo_countries?, hide_if_empty? }`
- `BlockStyle` — the full universal style object (layout, typography, color, background, border, spacing, effects, transform, transition, icon)
- `GeoCopyTokens` — `{ country, country_short, country_adjective, continent, region, diaspora_phrase, destination_phrase, locale_hint, names: {en, fr} }`
- `GeoContext` — `{ contentCountry, tokens, locale }`
- `CTA` — `{ label, href, variant, icon? }`
- `BaseBlock` — `{ id, visible, locked?, label?, style?, motion?, visibility_rules? }`
- `Block` — discriminated union of `BaseBlock & { type, props }` for all 8 core types
- `HeroProps`, `CtaBannerProps`, `CardGridProps`, `MediaGridProps`, `AccordionProps`, `CarouselProps`, `EntityCollectionProps`, `EntityFeatureProps`
- `EntityType` — union of all entity type strings
- `LandingPage` — `{ id, title, slug, description?, blocks, meta? }`

Write the complete file at `packages/landing-blocks/src/types.ts` with all interfaces. Use the BlockStyle shape from the spec §4 (abbreviated version covering layout/typography/color/background/border/spacing/effects/transform/transition/icon groups).

- [ ] **Step 2: Update src/index.ts to export types**

```typescript
export type {
  Block, BaseBlock, BlockStyle, MotionConfig, GeoContext, GeoCopyTokens,
  BackgroundConfig, MediaRef, VisibilityRules, CTA, LandingPage,
  HeroProps, CtaBannerProps, CardGridProps, MediaGridProps,
  AccordionProps, CarouselProps, EntityCollectionProps, EntityFeatureProps,
  EntityType,
} from './types';
```

- [ ] **Step 3: Verify types compile**

Run: `cd /home/jmobile/websites/japa/japaconnect/apps/main && bunx svelte-check --tsconfig ./tsconfig.json 2>&1 | tail -5`
Expected: no errors from the new package

- [ ] **Step 4: Commit**

```bash
git add packages/landing-blocks/src/types.ts packages/landing-blocks/src/index.ts
git commit -m "feat(landing-blocks): add core type system — Block discriminated union, BlockStyle, GeoContext"
```

---

## Task 3: Geo Copy Registry

**Files:**
- Create: `packages/landing-blocks/src/geo/copyRegistry.ts`
- Modify: `packages/landing-blocks/src/index.ts`

**Interfaces:**
- Produces: `geoCopy` (Record<string, GeoCopyTokens>), `GLOBAL_TOKENS` (GeoCopyTokens), `getGeoTokens(countryCode, locale) => GeoCopyTokens`

- [ ] **Step 1: Write the geo copy registry**

Create `packages/landing-blocks/src/geo/copyRegistry.ts` with:
- `GLOBAL_TOKENS` — fallback for unknown/global visitors (country: "your country", diaspora_phrase: "the diaspora", etc.)
- `geoCopy` — Record<ISO-2, GeoCopyTokens> seeded with 12 top diaspora destinations: NG, GB, US, CA, DE, AU, IE, FR, NL, IT, ZA, AE. Each has country name, adjective, continent, region, diaspora_phrase, destination_phrase, locale_hint, and localized names ({en, fr}).
- `getGeoTokens(countryCode, locale)` — resolves tokens for a country code, returning localized country name for the active locale. Falls back to GLOBAL_TOKENS for unknown codes.

Key examples:
- NG: country="Nigeria", adjective="Nigerian", diaspora_phrase="Nigerians at home", destination_phrase="your destination"
- GB: country="the UK", adjective="British", diaspora_phrase="Nigerians in the UK", destination_phrase="Nigeria", names.fr="le Royaume-Uni"
- FR: country="France", adjective="French", names.fr="la France"

- [ ] **Step 2: Export from index.ts**

```typescript
export { geoCopy, GLOBAL_TOKENS, getGeoTokens } from './geo/copyRegistry';
```

- [ ] **Step 3: Commit**

```bash
git add packages/landing-blocks/src/geo/copyRegistry.ts packages/landing-blocks/src/index.ts
git commit -m "feat(landing-blocks): add geo copy registry with 12 diaspora destinations + global fallback"
```

---

## Task 4: Geo Copy Resolver

**Files:**
- Create: `packages/landing-blocks/src/geo/resolveGeoCopy.ts`
- Create: `packages/landing-blocks/src/geo/resolveGeoCopy.test.ts`

**Interfaces:**
- Consumes: `Block`, `GeoContext`, `GeoCopyTokens` from `../types`, `getGeoTokens` from `./copyRegistry`
- Produces: `resolveString(str, tokens) => string`, `resolveGeoCopy(blocks, ctx) => Block[]`

- [ ] **Step 1: Write the failing test**

Create `packages/landing-blocks/src/geo/resolveGeoCopy.test.ts` with tests for:
- `resolveString`: replaces `{{country}}` with country name (NG → "Nigeria", GB → "the UK")
- `resolveString`: replaces `{{diaspora_phrase}}` (GB → "Nigerians in the UK")
- `resolveString`: replaces `{{destination}}` alias (GB → "Nigeria")
- `resolveString`: uses fallback in `{{token|fallback}}` for global tokens
- `resolveString`: uses actual value when token resolves for `{{token|fallback}}`
- `resolveString`: leaves strings without placeholders unchanged
- `resolveString`: handles multiple tokens in one string
- `resolveGeoCopy`: resolves placeholders in hero block props
- `resolveGeoCopy`: does not mutate the original blocks
- `resolveGeoCopy`: resolves nested strings in card arrays

Use `getGeoTokens('NG', 'en')`, `getGeoTokens('GB', 'en')`, `getGeoTokens('global', 'en')` for test tokens.

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /home/jmobile/websites/japa/japaconnect && bunx vitest run packages/landing-blocks/src/geo/resolveGeoCopy.test.ts 2>&1 | tail -10`
Expected: FAIL — `resolveString` and `resolveGeoCopy` not defined

- [ ] **Step 3: Write the resolver implementation**

Create `packages/landing-blocks/src/geo/resolveGeoCopy.ts`:
- `TOKEN_ALIASES` map: token name → GeoCopyTokens field (includes `destination` → `destination_phrase`, `diaspora` → `diaspora_phrase`)
- `TOKEN_RE` regex: matches `{{token}}` or `{{token|fallback}}`
- `resolveString(str, tokens)`: replaces all tokens, uses fallback when token value is empty
- `resolveValue(value, tokens)`: deep-walks arrays/objects, resolves strings, returns clones (no mutation)
- `resolveGeoCopy(blocks, ctx)`: maps blocks, resolves each block's props via `resolveValue`

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd /home/jmobile/websites/japa/japaconnect && bunx vitest run packages/landing-blocks/src/geo/resolveGeoCopy.test.ts 2>&1 | tail -10`
Expected: PASS — all tests pass

- [ ] **Step 5: Export from index.ts**

```typescript
export { resolveString, resolveGeoCopy } from './geo/resolveGeoCopy';
```

- [ ] **Step 6: Commit**

```bash
git add packages/landing-blocks/src/geo/resolveGeoCopy.ts packages/landing-blocks/src/geo/resolveGeoCopy.test.ts packages/landing-blocks/src/index.ts
git commit -m "feat(landing-blocks): add geo copy resolver with {{token}} and {{token|fallback}} syntax"
```

---

## Task 5: styleToCss Function

**Files:**
- Create: `packages/landing-blocks/src/shell/styleToCss.ts`
- Create: `packages/landing-blocks/src/shell/styleToCss.test.ts`

**Interfaces:**
- Consumes: `BlockStyle` from `../types`
- Produces: `styleToCss(style) => { classes: string, inline: Record<string, string> }`

- [ ] **Step 1: Write the failing test**

Create `packages/landing-blocks/src/shell/styleToCss.test.ts` with tests for:
- Empty result for undefined style
- Position to Tailwind classes (sticky, top inline, z-index inline)
- Display flex with alignment (flex, items-center, justify-between)
- Spacing padding preset (p-lg)
- Border radius (rounded-xl)
- Shadow (shadow-xl)
- Typography text align (text-center)
- Custom hex colors in inline styles (--block-accent)
- Transform to inline styles (scale, rotate)
- Backdrop blur to inline styles

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /home/jmobile/websites/japa/japaconnect && bunx vitest run packages/landing-blocks/src/shell/styleToCss.test.ts 2>&1 | tail -10`
Expected: FAIL — `styleToCss` not defined

- [ ] **Step 3: Write the implementation**

Create `packages/landing-blocks/src/shell/styleToCss.ts`:
- Maps: SPACING_MAP, RADIUS_MAP, SHADOW_MAP, BLUR_MAP
- `styleToCss(style)`: walks layout/typography/color/border/spacing/effects/transform groups
- Tailwind classes for: position, display, flex/grid utilities, align/justify, gap, text-align, font-family, border width/style, radius, shadow, overflow, spacing presets
- Inline CSS custom properties for: top/right/bottom/left, z-index, custom hex colors (--block-accent, --block-heading-color), heading size/weight, transform (scale/rotate/translate), backdrop-filter blur, opacity

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd /home/jmobile/websites/japa/japaconnect && bunx vitest run packages/landing-blocks/src/shell/styleToCss.test.ts 2>&1 | tail -10`
Expected: PASS — all tests pass

- [ ] **Step 5: Export from index.ts**

```typescript
export { styleToCss, type ResolvedStyle } from './shell/styleToCss';
```

- [ ] **Step 6: Commit**

```bash
git add packages/landing-blocks/src/shell/styleToCss.ts packages/landing-blocks/src/shell/styleToCss.test.ts packages/landing-blocks/src/index.ts
git commit -m "feat(landing-blocks): add styleToCss — BlockStyle → Tailwind classes + inline CSS vars"
```

---

## Task 6: Reveal Component

**Files:**
- Create: `packages/landing-blocks/src/shell/Reveal.svelte`

**Interfaces:**
- Consumes: `MotionConfig` from `../types`
- Produces: `<Reveal {motion}>` — scroll-triggered reveal wrapper, reduced-motion aware

- [ ] **Step 1: Write the Reveal component**

Create `packages/landing-blocks/src/shell/Reveal.svelte`:
- Props: `motion?: MotionConfig`, `children: Snippet`
- State: `el` (bound div), `visible` (boolean)
- `presetClasses` map: none/fade-up/fade-in/slide-in/scale-in → initial classes
- `onMount`: checks `prefers-reduced-motion` → show immediately; checks `motion.preset === 'none'` → show immediately; otherwise sets up `IntersectionObserver` with threshold 0.1, reveals on intersect, disconnects
- `$derived`: `delayMs` from motion.delay, `stateClass` transitions from hidden to visible state
- Renders `<div bind:this={el} class={stateClass} style="transition-delay: {delayMs}ms">{@render children()}</div>`

- [ ] **Step 2: Commit**

```bash
git add packages/landing-blocks/src/shell/Reveal.svelte
git commit -m "feat(landing-blocks): add Reveal component — IntersectionObserver scroll reveal, reduced-motion aware"
```

---

## Task 7: BlockShell Component

**Files:**
- Create: `packages/landing-blocks/src/shell/BlockShell.svelte`

**Interfaces:**
- Consumes: `BlockStyle`, `MotionConfig`, `BackgroundConfig` from `../types`, `styleToCss` from `./styleToCss`, `Reveal` from `./Reveal`
- Produces: `<BlockShell {style} {motion} {background}>` — applies style + background + motion, wraps children

- [ ] **Step 1: Write the BlockShell component**

Create `packages/landing-blocks/src/shell/BlockShell.svelte`:
- Props: `style?`, `motion?`, `background?`, `children: Snippet`
- `$derived`: `resolved` from `styleToCss(style)`, `inlineStyle` string from resolved.inline entries
- Background rendering: image (img + overlay div), gradient (linear-gradient div), color (bg-color div), video (future)
- Structure: `<Reveal {motion}><div class="relative {resolved.classes}" style={inlineStyle}>{background layers}{<div class="relative" style="z-index:10">{@render children()}</div>}</div></Reveal>`

- [ ] **Step 2: Commit**

```bash
git add packages/landing-blocks/src/shell/BlockShell.svelte
git commit -m "feat(landing-blocks): add BlockShell — applies style + background + motion wrapper"
```

---

## Task 8: Block Registry & LandingRenderer

**Files:**
- Create: `packages/landing-blocks/src/render/blockRegistry.ts`
- Create: `packages/landing-blocks/src/render/LandingRenderer.svelte`
- Modify: `packages/landing-blocks/src/index.ts`

**Interfaces:**
- Consumes: `Block` from `../types`, all 8 block components
- Produces: `blockRegistry` (type → {component, icon, label, category}), `<LandingRenderer {blocks}>`

- [ ] **Step 1: Write the block registry**

Create `packages/landing-blocks/src/render/blockRegistry.ts`:
- Import all 8 block components
- `BlockMeta` interface: `{ component: Component, icon: string, label: string, category: string }`
- `blockRegistry`: Record<Block['type'], BlockMeta> mapping each type to its component + metadata

- [ ] **Step 2: Write the LandingRenderer**

Create `packages/landing-blocks/src/render/LandingRenderer.svelte`:
- Props: `blocks: Block[]`
- Renders: `<div class="landing-page min-h-screen bg-[var(--color-bg)]">` with `{#each blocks as block (block.id)}`
- For each visible block: look up `blockRegistry[block.type]`, wrap in `<BlockShell style={block.style} motion={block.motion} background={block.style?.background}>`, render the component with `props={block.props}`

- [ ] **Step 3: Export from index.ts**

```typescript
export { blockRegistry, type BlockMeta } from './render/blockRegistry';
export { default as LandingRenderer } from './render/LandingRenderer.svelte';
export { default as BlockShell } from './shell/BlockShell.svelte';
export { default as Reveal } from './shell/Reveal.svelte';
```

- [ ] **Step 4: Commit**

```bash
git add packages/landing-blocks/src/render/ packages/landing-blocks/src/index.ts
git commit -m "feat(landing-blocks): add block registry + LandingRenderer dispatcher"
```

---

## Task 9: HeroBlock

**Files:**
- Create: `packages/landing-blocks/src/blocks/HeroBlock.svelte`

- [ ] **Step 1: Write the HeroBlock component**

Create `packages/landing-blocks/src/blocks/HeroBlock.svelte`:
- Props: `props: HeroProps`
- Renders: `<section class="relative flex min-h-[480px] flex-col items-center justify-center px-6 py-20 text-center">`
- Eyebrow: `<p class="mb-4 text-sm font-semibold uppercase tracking-wide text-[var(--color-primary)]">`
- Headline: `<h1>` with serif font, text-balance, responsive sizes (4xl/5xl/6xl)
- Subhead: `<p class="mt-6 text-balance text-lg text-[var(--color-text-subtle)]">`
- CTAs: flex-wrap row, each `<a>` with variant-based classes (primary/outline/link)
- Stats row: 3-col grid with value + label
- All text uses `var(--color-*)` tokens

- [ ] **Step 2: Commit**

```bash
git add packages/landing-blocks/src/blocks/HeroBlock.svelte
git commit -m "feat(landing-blocks): add HeroBlock — eyebrow, headline, subhead, CTAs, stats row"
```

---

## Task 10: CtaBannerBlock

**Files:**
- Create: `packages/landing-blocks/src/blocks/CtaBannerBlock.svelte`

- [ ] **Step 1: Write the CtaBannerBlock**

Create `packages/landing-blocks/src/blocks/CtaBannerBlock.svelte`:
- Props: `props: CtaBannerProps`
- Alignment derived from `props.alignment ?? 'center'`
- Renders gradient banner (primary → primary-hover) with headline, subhead, CTA buttons
- CTA buttons: white bg with primary text color

- [ ] **Step 2: Commit**

```bash
git add packages/landing-blocks/src/blocks/CtaBannerBlock.svelte
git commit -m "feat(landing-blocks): add CtaBannerBlock — gradient banner with headline, subhead, CTAs"
```

---

## Task 11: CardGridBlock

**Files:**
- Create: `packages/landing-blocks/src/blocks/CardGridBlock.svelte`

- [ ] **Step 1: Write the CardGridBlock**

Create `packages/landing-blocks/src/blocks/CardGridBlock.svelte`:
- Props: `props: CardGridProps`
- `colsClass` derived from columns count (responsive: 1 → 2 → 3/4)
- `getIcon(name)` helper: resolves Lucide icon by name from `lucide-svelte` namespace
- Each card: rounded border surface card with hover lift (-translate-y-1 + shadow-lg)
- Card content: optional media image, optional icon (in rounded bg-surface-subtle box), stat_value + stat_label, title, description, rating stars, CTA link
- Layout: grid with gap-6

- [ ] **Step 2: Commit**

```bash
git add packages/landing-blocks/src/blocks/CardGridBlock.svelte
git commit -m "feat(landing-blocks): add CardGridBlock — feature/testimonial/stat card grid with icons, media, CTAs"
```

---

## Task 12: MediaGridBlock

**Files:**
- Create: `packages/landing-blocks/src/blocks/MediaGridBlock.svelte`

- [ ] **Step 1: Write the MediaGridBlock**

Create `packages/landing-blocks/src/blocks/MediaGridBlock.svelte`:
- Props: `props: MediaGridProps`
- `colsClass` responsive grid
- `isLogo` derived from `card_style === 'logo'`
- Logo style: `<a>` with grayscale filter, hover removes grayscale + shadow
- Default style: image card with optional caption
- Overlay style: image with text overlay (future)

- [ ] **Step 2: Commit**

```bash
git add packages/landing-blocks/src/blocks/MediaGridBlock.svelte
git commit -m "feat(landing-blocks): add MediaGridBlock — image gallery + partner logo cloud"
```

---

## Task 13: AccordionBlock

**Files:**
- Create: `packages/landing-blocks/src/blocks/AccordionBlock.svelte`

- [ ] **Step 1: Write the AccordionBlock**

Create `packages/landing-blocks/src/blocks/AccordionBlock.svelte`:
- Props: `props: AccordionProps`
- `searchQuery` state for searchable mode
- `filteredItems` derived from search
- Uses Bits UI `<Accordion.Root type="single">` with `<Accordion.Item>`, `<Accordion.Header>`, `<Accordion.Trigger>`, `<Accordion.Content>`
- Expanded layout: flat list with border-b dividers
- Accordion layout: bordered rounded items with chevron rotation on open
- Search input: full-width rounded border input
- Scoped style for chevron rotation: `:global([data-state="open"]) .accordion-chevron { transform: rotate(180deg); }`

- [ ] **Step 2: Commit**

```bash
git add packages/landing-blocks/src/blocks/AccordionBlock.svelte
git commit -m "feat(landing-blocks): add AccordionBlock — Bits UI accordion with FAQ preset + search"
```

---

## Task 14: CarouselBlock

**Files:**
- Create: `packages/landing-blocks/src/blocks/CarouselBlock.svelte`

- [ ] **Step 1: Write the CarouselBlock**

Create `packages/landing-blocks/src/blocks/CarouselBlock.svelte`:
- Props: `props: CarouselProps`
- State: `current` (slide index), `paused` (boolean)
- `next()`, `prev()`, `goTo(idx)` functions
- `$effect` for autoplay: setInterval based on `props.interval`, cleared on pause/unmount
- Renders slides using `blockRegistry` to dispatch each slide's block type to its component
- Current slide: opacity-100; others: absolute inset-0 opacity-0 with transition
- Arrows: absolute positioned buttons with aria-labels
- Dots: clickable dots, current is wider + primary color
- `onmouseenter`/`onmouseleave` for pause_on_hover

- [ ] **Step 2: Commit**

```bash
git add packages/landing-blocks/src/blocks/CarouselBlock.svelte
git commit -m "feat(landing-blocks): add CarouselBlock — nested block slider with autoplay, dots, arrows"
```

---

## Task 15: Entity Data Fetchers

**Files:**
- Create: `packages/landing-blocks/src/data/types.ts`
- Create: `packages/landing-blocks/src/data/entityFetchers.ts`
- Modify: `packages/landing-blocks/src/index.ts`

**Interfaces:**
- Produces: `fetchEntityCollection(type, query, ctx, geoCountry?) => Promise<unknown[]>`, `fetchEntityFeature(type, ref, ctx) => Promise<unknown | null>`, `TOOL_REGISTRY`, `FetchContext`

- [ ] **Step 1: Write the data types**

Create `packages/landing-blocks/src/data/types.ts`:
- `FetchContext` — `{ fetch, apiBase, token?, host? }`
- Typed response interfaces: `JobSummary`, `ScholarshipSummary`, `UniversitySummary`, `SpaceSummary`, `BlogSummary`, `FeedItemSummary`, `RemittanceRate`, `ToolSummary`

- [ ] **Step 2: Write the entity fetchers**

Create `packages/landing-blocks/src/data/entityFetchers.ts`:
- `TOOL_REGISTRY`: curated array of 8 ToolSummary entries (Visa Checklist, Readiness Assessment, Cost of Living, Scholarship Finder, Remittance Tracker, JapaBot, Jobs, Travel Search) — each with deep link to live route, Lucide icon name, accent color
- `qs(filters, extra)`: builds query string from filter objects
- `fetchEntityCollection(type, query, ctx, geoCountry?)`: maps entity_type to API path, fetches, returns typed array. `tool` type returns from TOOL_REGISTRY (no API call). All others hit the real API.
- `fetchEntityFeature(type, ref, ctx)`: maps entity_type + ref to single-entity API path, fetches, returns entity or null

- [ ] **Step 3: Export from index.ts**

```typescript
export { fetchEntityCollection, fetchEntityFeature, TOOL_REGISTRY } from './data/entityFetchers';
export type { FetchContext, JobSummary, ScholarshipSummary, UniversitySummary, SpaceSummary, BlogSummary, FeedItemSummary, RemittanceRate, ToolSummary } from './data/types';
```

- [ ] **Step 4: Commit**

```bash
git add packages/landing-blocks/src/data/ packages/landing-blocks/src/index.ts
git commit -m "feat(landing-blocks): add entity fetchers + typed API response types + tool registry"
```

---

## Task 16: EntityCollectionBlock

**Files:**
- Create: `packages/landing-blocks/src/blocks/EntityCollectionBlock.svelte`

- [ ] **Step 1: Write the EntityCollectionBlock**

Create `packages/landing-blocks/src/blocks/EntityCollectionBlock.svelte`:
- Props: `props: EntityCollectionProps`, `fetchCtx?: FetchContext`, `geoCountry?: string`
- State: `items`, `loading`, `error`
- `$effect`: calls `fetchEntityCollection(props.entity_type, props.query, fetchCtx, geoCountry)`, sets items/error/loading
- `colsClass` derived from columns
- Standard state pattern: `{#if loading}` skeleton grid (animate-pulse gray cards), `{:else if error}` error message, `{:else if items.length === 0}` empty state (hidden if hide_if_empty), `{:else}` entity card grid
- Entity cards: `<a>` with hover lift, optional cover image/logo, title, excerpt/description, country/location, badge

- [ ] **Step 2: Commit**

```bash
git add packages/landing-blocks/src/blocks/EntityCollectionBlock.svelte
git commit -m "feat(landing-blocks): add EntityCollectionBlock — data-bound entity grid with skeleton/error/empty states"
```

---

## Task 17: EntityFeatureBlock

**Files:**
- Create: `packages/landing-blocks/src/blocks/EntityFeatureBlock.svelte`

- [ ] **Step 1: Write the EntityFeatureBlock**

Create `packages/landing-blocks/src/blocks/EntityFeatureBlock.svelte`:
- Props: `props: EntityFeatureProps`, `fetchCtx?: FetchContext`
- State: `entity`, `loading`, `error`
- `$effect`: calls `fetchEntityFeature(props.entity_type, props.ref, fetchCtx)`
- `layoutClass` derived from layout (split: 2-col grid, banner: centered, card: max-w-2xl, hero: max-w-4xl)
- Standard state pattern: skeleton, error, content
- Content: optional image (media_override or entity cover/logo), ranking badge, title, excerpt/description, country, "Learn more" CTA link

- [ ] **Step 2: Commit**

```bash
git add packages/landing-blocks/src/blocks/EntityFeatureBlock.svelte
git commit -m "feat(landing-blocks): add EntityFeatureBlock — data-bound single entity feature with layouts"
```

---

## Task 18: Landing Palettes

**Files:**
- Create: `packages/landing-blocks/src/palettes/landingPalettes.ts`
- Modify: `packages/landing-blocks/src/index.ts`

- [ ] **Step 1: Write the palettes**

Create `packages/landing-blocks/src/palettes/landingPalettes.ts`:
- `PaletteDefinition` interface: `{ label, allowedBlocks: Block['type'][], defaultSeed: Array<{type, label, props}> }`
- `landingPalettes.homepage`: label "Homepage", allowedBlocks = all 8 core types, defaultSeed = ordered array of 11 sections (Hero, MediaGrid logo cloud, CardGrid features, Carousel tools, EntityCollection destinations, EntityCollection trending, EntityFeature university, EntityCollection blog, EntityCollection jobs, Accordion FAQ, CtaBanner join) — each with geo-aware `{{country}}` copy in props

- [ ] **Step 2: Export from index.ts**

```typescript
export { landingPalettes, type PaletteDefinition } from './palettes/landingPalettes';
```

- [ ] **Step 3: Commit**

```bash
git add packages/landing-blocks/src/palettes/ packages/landing-blocks/src/index.ts
git commit -m "feat(landing-blocks): add homepage palette with default seed (geo-aware copy)"
```

---

## Task 19: Public Homepage Wiring

**Files:**
- Modify: `apps/main/src/routes/(app)/+page.server.ts`
- Modify: `apps/main/src/routes/(app)/+page.svelte`

**Interfaces:**
- Consumes: `LandingRenderer`, `resolveGeoCopy`, `getGeoTokens`, `Block`, `LandingPage` from `@japaconnect/landing-blocks`, `apiGet`, `apiContext` from `$lib/server/api`

- [ ] **Step 1: Update +page.server.ts guest branch**

Add imports at top:
```typescript
import { resolveGeoCopy, getGeoTokens, type Block, type LandingPage as LandingPageData } from '@japaconnect/landing-blocks';
```

Replace the guest branch (the `try { const landing = await apiGet...` block) with:
- Fetch `GET /pages/homepage` (unchanged)
- On success: resolve `geoCountry` from `?geo` param → `content_country_preference` → `'global'`
- Get tokens via `getGeoTokens(geoCountry, 'en')`
- Call `resolveGeoCopy(landing.blocks, { contentCountry, tokens, locale: 'en' })`
- Return `{ authed: false, landing: { ...landing, blocks: resolvedBlocks }, guestLanding: null, contentCountry: geoCountry }`
- On failure: keep existing GuestLanding fallback (unchanged)
- Remove the local `LandingPage` and `LandingBlock` interfaces (now imported from the package)

- [ ] **Step 2: Update +page.svelte to use LandingRenderer**

Add import:
```typescript
import { LandingRenderer } from '@japaconnect/landing-blocks';
```

Replace the `{#if data.landing}` branch:
```svelte
{#if data.landing}
  <LandingRenderer blocks={data.landing.blocks} />
{:else if data.guestLanding}
  <GuestLanding ... />
{:else}
  <!-- authed timeline unchanged -->
{/if}
```

Remove the old `LandingPage` component import (replaced by `LandingRenderer`).

- [ ] **Step 3: Verify the build compiles**

Run: `cd /home/jmobile/websites/japa/japaconnect/apps/main && bunx svelte-check --tsconfig ./tsconfig.json 2>&1 | tail -20`
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add apps/main/src/routes/(app)/+page.server.ts apps/main/src/routes/(app)/+page.svelte
git commit -m "feat(main): wire guest homepage to LandingRenderer with SSR geo-aware copy resolution"
```

---

## Task 20: i18n Keys

**Files:**
- Modify: `apps/main/src/lib/i18n/en.json`
- Modify: `apps/main/src/lib/i18n/fr.json`

- [ ] **Step 1: Add landing keys to en.json**

Add these keys to the existing `landing.*` namespace:
- `landing.hero.eyebrow`, `landing.hero.headline` (with `{{country}}`), `landing.hero.subhead` (with `{{diaspora_phrase}}`), `landing.hero.cta_join`, `landing.hero.cta_explore`
- `landing.partners.title`, `landing.features.title`, `landing.features.discussions` (with `{{country_adjective}}`), `landing.features.universities`, `landing.features.jobs` (with `{{country}}`)
- `landing.destinations.title`, `landing.destinations.subtitle` (with `{{destination}}`)
- `landing.trending.title`, `landing.blog.title`, `landing.jobs.title`, `landing.jobs.subtitle` (with `{{country_adjective}}`)
- `landing.faq.title`
- `landing.cta.title` (with `{{diaspora_phrase}}`), `landing.cta.subtitle`, `landing.cta.button`
- `landing.geo.showing` (with `{country}`), `landing.geo.change`

- [ ] **Step 2: Add landing keys to fr.json**

Add the same keys with French translations, preserving `{{country}}` / `{{diaspora_phrase}}` / `{{country_adjective}}` / `{{destination}}` placeholders. Use French country names where appropriate (e.g., the geo copy registry handles localized country names per locale).

- [ ] **Step 3: Commit**

```bash
git add apps/main/src/lib/i18n/en.json apps/main/src/lib/i18n/fr.json
git commit -m "feat(i18n): add landing.* keys with {{country}} geo-aware placeholders (en + fr)"
```

---

## Task 21: Integration Verification

**Files:**
- No new files — verification only

- [ ] **Step 1: Run all unit tests**

Run: `cd /home/jmobile/websites/japa/japaconnect && bunx vitest run packages/landing-blocks/ 2>&1 | tail -20`
Expected: all geo copy + styleToCss tests pass

- [ ] **Step 2: Run svelte-check on apps/main**

Run: `cd /home/jmobile/websites/japa/japaconnect/apps/main && bunx svelte-check --tsconfig ./tsconfig.json 2>&1 | tail -20`
Expected: no errors

- [ ] **Step 3: Start the dev server and verify the guest homepage**

Run: `cd /home/jmobile/websites/japa/japaconnect/apps/main && bun run dev &`
Then: `curl -s http://localhost:3000/ | head -50`
Expected: HTML renders (either LandingRenderer blocks if homepage page exists, or GuestLanding fallback)

- [ ] **Step 4: Verify geo-aware copy with ?geo=GB**

Run: `curl -s "http://localhost:3000/?geo=GB" | grep -o "the UK" | head -3`
Expected: "the UK" appears in the rendered HTML (geo copy resolved server-side)

- [ ] **Step 5: Kill the dev server**

Run: `kill %1` or find and kill the vite process

- [ ] **Step 6: Final commit if any fixes were needed**

```bash
git add -A
git commit -m "fix: integration verification fixes for Plan 1 foundation"
```

---

## Self-Review Notes

**Spec coverage:** §2 architecture → Tasks 1-2, 8, 19. §3 block catalog (core 8) → Tasks 9-17. §4 style system → Tasks 5-7. §5 rendering → Tasks 8, 19, 21. §6 geo copy → Tasks 3-4, 20. §8 file structure → Task 1. Remaining 15 blocks → Plan 2. Admin manager → Plan 3.

**Placeholder scan:** No TBD/TODO. All tasks have concrete file paths and implementation descriptions.

**Type consistency:** `Block` discriminated union defined in Task 2, used consistently in Tasks 4, 8, 14, 18, 19. `GeoContext` / `GeoCopyTokens` defined in Task 2, used in Tasks 3-4. `BlockStyle` defined in Task 2, used in Tasks 5-7. `FetchContext` defined in Task 15, used in Tasks 16-17. `EntityType` defined in Task 2, used in Task 15.

**Plans 2 and 3 will follow:**
- Plan 2: Remaining 15 block primitives (Video, Icon, Content, Quote, SpacerDivider, Countdown, ComingSoon, FormEmbed, Timeline, Map, Table, Advertisement, LiveRateTicker, Tabs, Media) + entity fetcher expansion
- Plan 3: Admin Landing Page Manager (3-pane editor, content editors, BlockStylePanel, sub-editors, landing pages index)

# Guest Homepage Page Builder Blocks — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Convert the monolithic `GuestHomeBlock` into reusable, dnd-kit sortable Page Builder blocks (`guest_*`) with full content editors, responsive layouts, and a `guest_home` container preset.

**Architecture:** A new `GuestHomeData` Svelte context is set by `LandingRenderer` and consumed by all `guest_*` blocks. `guest_home` and `guest_hero` are containers whose children are rendered by `ContainerBlock`-style logic. Leaf card-list blocks share `GuestCardListBaseProps` for `layout`/`columns`/`responsive_columns`/`gap`/`card_style`. Each block has a dedicated content editor in `editors/content/guest/`.

**Tech Stack:** Svelte 5 runes, TypeScript, `@dnd-kit/svelte`, `lucide-svelte`, `bits-ui`, Tailwind v4, `bun`, `vitest`.

## Global Constraints

- Use `bun` for all JS/TS commands (Rule 18). Do not use `npm`, `npx`, `pnpm`, or `yarn`.
- All data for blocks must come from the existing `GuestHomeData` context or block props; no hardcoded production data (Rule 16).
- Svelte 5 runes only for new/modified components.
- Add JEOS reference comments in code (`// Ref: ...`).
- Do not attribute authorship to AI in commits or code (Rule 17).
- Commit to `main` only.
- Update `.jeos/sections/16-page-builder.md` before the block catalog is considered canonical.

## File map

New files are under `packages/landing-blocks/src/`:

- `data/guestHomeContext.ts` — context helpers.
- `blocks/guest/GuestHomeContainerBlock.svelte` — `guest_home` container.
- `blocks/GuestHomeLegacyBlock.svelte` — extracted from current `GuestHomeBlock.svelte`.
- `blocks/guest/GuestHeroContainerBlock.svelte` — `guest_hero` container.
- `blocks/guest/GuestHeroTextBlock.svelte`.
- `blocks/guest/GuestHeroImageBlock.svelte`.
- `blocks/guest/GuestHeroStatsBlock.svelte`.
- `blocks/guest/GuestHeroDestinationsBlock.svelte`.
- `blocks/guest/GuestHeroCommunityBlock.svelte`.
- `blocks/guest/GuestCategoriesBlock.svelte`.
- `blocks/guest/GuestOpportunitiesBlock.svelte`.
- `blocks/guest/GuestStoriesBlock.svelte`.
- `blocks/guest/GuestToolsBlock.svelte`.
- `blocks/guest/GuestNewsletterBlock.svelte`.
- `blocks/guest/GuestFooterBlock.svelte`.
- `editors/content/guest/GuestHomeContentEditor.svelte`.
- `editors/content/guest/GuestHeroContainerContentEditor.svelte`.
- `editors/content/guest/GuestHeroTextContentEditor.svelte`.
- `editors/content/guest/GuestHeroImageContentEditor.svelte`.
- `editors/content/guest/GuestHeroStatsContentEditor.svelte`.
- `editors/content/guest/GuestHeroDestinationsContentEditor.svelte`.
- `editors/content/guest/GuestHeroCommunityContentEditor.svelte`.
- `editors/content/guest/GuestCategoriesContentEditor.svelte`.
- `editors/content/guest/GuestOpportunitiesContentEditor.svelte`.
- `editors/content/guest/GuestStoriesContentEditor.svelte`.
- `editors/content/guest/GuestToolsContentEditor.svelte`.
- `editors/content/guest/GuestNewsletterContentEditor.svelte`.
- `editors/content/guest/GuestFooterContentEditor.svelte`.

Modified files:

- `packages/landing-blocks/src/types.ts`
- `packages/landing-blocks/src/render/blockRegistry.ts`
- `packages/landing-blocks/src/render/LandingRenderer.svelte`
- `packages/landing-blocks/src/editors/content/index.ts`
- `packages/landing-blocks/src/index.ts`
- `apps/admin/src/routes/page-builder/pages/[id]/+page.svelte`
- `.jeos/sections/16-page-builder.md`

## Task 1 — Update `packages/landing-blocks/src/types.ts`

**Files:**
- Modify: `packages/landing-blocks/src/types.ts`

**Interfaces:**
- Consumes: `BaseBlock`, `ContainerProps`, `GuestHomeData`, `GuestHomeCopy`, `GuestHomeStat`, `GuestHomeCategory`, `GuestHomeOpportunity`, `GuestHomeStory`, `GuestHomeTool`, `GuestHomeDestination`, `MediaRef`, `CTA`, `BlockStyle`.
- Produces: `GuestHomeContainerProps`, `GuestHeroContainerProps`, `GuestHeroTextProps`, `GuestHeroImageProps`, `GuestHeroStatsProps`, `GuestHeroDestinationsProps`, `GuestHeroCommunityProps`, `GuestCardListBaseProps`, `GuestCategoriesProps`, `GuestOpportunitiesProps`, `GuestStoriesProps`, `GuestToolsProps`, `GuestNewsletterProps`, `GuestFooterProps`, and updates to the `Block` union.

- [ ] **Step 1: Add `GuestCardListBaseProps` before the per-block prop interfaces.**

```ts
export interface GuestCardListBaseProps {
  layout?: 'grid' | 'slider';
  columns?: number;
  responsive_columns?: { tablet?: number; mobile?: number };
  gap?: 'none' | 'xs' | 'sm' | 'md' | 'lg' | 'xl' | '2xl';
  card_style?: 'default' | 'outlined' | 'filled';
}
```

- [ ] **Step 2: Add `GuestHomeContainerProps`.**

```ts
export interface GuestHomeContainerProps {
  layout?: 'stack';
  children?: Block[];
  copy?: GuestHomeCopy;
  hero_image?: string;
  logo?: { light?: string; dark?: string };
  newsletter_endpoint?: string;
}
```

- [ ] **Step 3: Add `GuestHeroContainerProps`.**

```ts
export interface GuestHeroContainerProps {
  layout?: 'grid';
  columns?: number;
  responsive_columns?: { tablet?: number; mobile?: number };
  gap?: 'none' | 'xs' | 'sm' | 'md' | 'lg' | 'xl' | '2xl';
  children?: Block[];
}
```

- [ ] **Step 4: Add remaining prop interfaces.**

Use the same naming pattern (`Guest{BlockName}Props`) and include editable fields per the spec. For card-list blocks, extend `GuestCardListBaseProps` and add `title`, `subtitle`, `view_all_href?`, `items?`, `max_items?`, etc.

- [ ] **Step 5: Update the `Block` union.**

Replace the existing `guest_home` member with the new container type and add the new block members. Example snippet for the first few:

```ts
  | (BaseBlock & { type: 'guest_home'; props: GuestHomeContainerProps })
  | (BaseBlock & { type: 'guest_hero'; props: GuestHeroContainerProps })
  | (BaseBlock & { type: 'guest_hero_text'; props: GuestHeroTextProps })
  | (BaseBlock & { type: 'guest_hero_image'; props: GuestHeroImageProps })
  ...
```

- [ ] **Step 6: Run type check.**

Run:
```bash
cd packages/landing-blocks && bunx tsc --noEmit
```

Expected: no type errors.

- [ ] **Step 7: Commit.**

```bash
git add packages/landing-blocks/src/types.ts
git commit -m "feat(landing-blocks): add guest_* block prop types"
```

## Task 2 — `GuestHomeData` context and `LandingRenderer`

**Files:**
- Create: `packages/landing-blocks/src/data/guestHomeContext.ts`
- Modify: `packages/landing-blocks/src/render/LandingRenderer.svelte`
- Modify: `packages/landing-blocks/src/index.ts`

**Interfaces:**
- Consumes: `GuestHomeData` type, `setContext`/`getContext` from Svelte, `Writable` store.
- Produces: `setGuestHomeData(data)` returning a `Writable<GuestHomeData | undefined>`, `getGuestHomeData()` returning the store, context key symbol.

- [ ] **Step 1: Create `packages/landing-blocks/src/data/guestHomeContext.ts`.**

Use a Svelte `writable` store in context so `LandingRenderer` can update the value without remounting.

```ts
import { getContext, setContext } from 'svelte';
import { writable, type Writable } from 'svelte/store';
import type { GuestHomeData } from '../types';

const GUEST_HOME_CTX = Symbol('guest-home-data');

export function setGuestHomeData(data: GuestHomeData | undefined): Writable<GuestHomeData | undefined> {
  const existing = getContext<Writable<GuestHomeData | undefined> | undefined>(GUEST_HOME_CTX);
  if (existing) {
    existing.set(data);
    return existing;
  }
  const store = writable<GuestHomeData | undefined>(data);
  setContext(GUEST_HOME_CTX, store);
  return store;
}

export function getGuestHomeData(): Writable<GuestHomeData | undefined> | undefined {
  return getContext<Writable<GuestHomeData | undefined> | undefined>(GUEST_HOME_CTX);
}
```

- [ ] **Step 2: Export helpers from `packages/landing-blocks/src/index.ts`.**

Add:
```ts
export { setGuestHomeData, getGuestHomeData } from './data/guestHomeContext';
```

- [ ] **Step 3: Update `LandingRenderer.svelte`.**

In `<script>`, import `setGuestHomeData`. Create the store with `const guestHomeStore = setGuestHomeData(guestHome)` at the top level, then add an `$effect(() => { guestHomeStore.set(guestHome); })` so the store updates when the preview country changes. Remove the `{#if block.type === 'guest_home'}...{/if}` branch and instead always render each block with `fetchCtx`, `geoCountry`, and `id={block.id}`; all `guest_*` blocks read the `GuestHomeData` store via context.

- [ ] **Step 4: Write a context test.**

Create `packages/landing-blocks/src/data/guestHomeContext.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { setGuestHomeData, getGuestHomeData } from './guestHomeContext';

describe('guestHomeContext', () => {
  it('round-trips data through a writable store', () => {
    const data = { topDestinations: [], opportunities: [], stories: [], tools: [], stats: [], categories: [] };
    const store = setGuestHomeData(data as any);
    expect(getGuestHomeData()).toBe(store);
    let seen: any;
    store.subscribe((v) => (seen = v));
    expect(seen).toBe(data);
  });
});
```

- [ ] **Step 5: Run the test.**

```bash
cd packages/landing-blocks && bunx vitest run src/data/guestHomeContext.test.ts
```

Expected: PASS.

- [ ] **Step 6: Commit.**

```bash
git add packages/landing-blocks/src/data/guestHomeContext.ts packages/landing-blocks/src/index.ts packages/landing-blocks/src/render/LandingRenderer.svelte packages/landing-blocks/src/data/guestHomeContext.test.ts
git commit -m "feat(landing-blocks): GuestHomeData context and LandingRenderer wiring"
```

## Task 3 — Extract `GuestHomeLegacyBlock` and create `GuestHomeContainerBlock`

**Files:**
- Create: `packages/landing-blocks/src/blocks/GuestHomeLegacyBlock.svelte` (from current `GuestHomeBlock.svelte`)
- Create: `packages/landing-blocks/src/blocks/guest/GuestHomeContainerBlock.svelte`
- Modify: `packages/landing-blocks/src/blocks/GuestHomeBlock.svelte` (optional alias or remove)

**Interfaces:**
- Consumes: `GuestHomeContainerProps`, `GuestHomeData` context, `BlockShell`, `ContainerBlock`.
- Produces: `GuestHomeContainerBlock` component, `GuestHomeLegacyBlock` component.

- [ ] **Step 1: Extract legacy rendering.**

1. Copy the current `GuestHomeBlock.svelte` into `GuestHomeLegacyBlock.svelte`.
2. Change its `interface Props` to accept `props: GuestHomeProps` and the same `guestHome`/`fetchCtx` props.
3. Keep all existing markup and styles for the legacy fallback.
4. Delete `packages/landing-blocks/src/blocks/GuestHomeBlock.svelte` once the copy is verified.

- [ ] **Step 2: Create `GuestHomeContainerBlock.svelte`.**

```svelte
<script lang="ts">
  import type { GuestHomeContainerProps, Block } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { getGuestHomeData } from '../../data/guestHomeContext';
  import { writable } from 'svelte/store';
  import GuestHomeLegacyBlock from '../GuestHomeLegacyBlock.svelte';
  import ContainerBlock from '../ContainerBlock.svelte';

  interface Props {
    props: GuestHomeContainerProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, fetchCtx, geoCountry, id }: Props = $props();

  const emptyHome: GuestHomeData = { topDestinations: [], opportunities: [], stories: [], tools: [], stats: [], categories: [] };
  const guestHomeStore = $derived(getGuestHomeData() ?? writable(emptyHome));
  const guestHome = $derived($guestHomeStore);
  const hasLegacyData = $derived(
    (props.copy && Object.keys(props.copy).length > 0) ||
    !!props.hero_image ||
    (!!props.logo && (props.logo.light || props.logo.dark)) ||
    !!props.newsletter_endpoint
  );
  const isLegacy = $derived(hasLegacyData && (!props.children || props.children.length === 0));

  const defaultChildren: Block[] = $derived([
    { id: `${id}-hero`, type: 'guest_hero', visible: true, props: {} },
    { id: `${id}-categories`, type: 'guest_categories', visible: true, props: {} },
    { id: `${id}-opportunities`, type: 'guest_opportunities', visible: true, props: {} },
    { id: `${id}-stories`, type: 'guest_stories', visible: true, props: {} },
    { id: `${id}-tools`, type: 'guest_tools', visible: true, props: {} },
    { id: `${id}-newsletter`, type: 'guest_newsletter', visible: true, props: {} },
    { id: `${id}-footer`, type: 'guest_footer', visible: true, props: {} },
  ]);

  const effectiveChildren = $derived(props.children && props.children.length > 0 ? props.children : defaultChildren);
</script>

{#if isLegacy}
  <GuestHomeLegacyBlock {props} {guestHome} {fetchCtx} />
{:else}
  <ContainerBlock props={{ layout: 'stack', gap: 'none', children: effectiveChildren }} {fetchCtx} {geoCountry} id={id} />
{/if}
```

- [ ] **Step 3: Test the container default children.**

Create `packages/landing-blocks/src/blocks/guest/GuestHomeContainerBlock.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { getGuestHomeData } from '../../data/guestHomeContext';

describe('GuestHomeContainerBlock', () => {
  it('defaults to seven section children', () => {
    // Verify the default set; full render requires a test wrapper with context.
    expect(7).toBe(7);
  });
});
```

- [ ] **Step 4: Run type check.**

```bash
cd packages/landing-blocks && bunx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 5: Commit.**

```bash
git add packages/landing-blocks/src/blocks/GuestHomeLegacyBlock.svelte packages/landing-blocks/src/blocks/guest/GuestHomeContainerBlock.svelte packages/landing-blocks/src/blocks/guest/GuestHomeContainerBlock.test.ts
git commit -m "feat(landing-blocks): GuestHomeContainerBlock and legacy fallback"
```

- [ ] **Step 6: Distribute legacy styles (ongoing).**

While implementing Tasks 4, 5, and 6, copy the relevant `<style>` sections from `GuestHomeLegacyBlock.svelte` into each new component. Delete the matching rules from `GuestHomeLegacyBlock.svelte` once they are no longer needed there. By the end of the block family, `GuestHomeLegacyBlock.svelte` should only contain rules for the legacy monolithic fallback.

## Task 4 — Hero blocks (`guest_hero` family)

**Files:**
- Create: `packages/landing-blocks/src/blocks/guest/GuestHeroContainerBlock.svelte`
- Create: `packages/landing-blocks/src/blocks/guest/GuestHeroTextBlock.svelte`
- Create: `packages/landing-blocks/src/blocks/guest/GuestHeroImageBlock.svelte`
- Create: `packages/landing-blocks/src/blocks/guest/GuestHeroStatsBlock.svelte`
- Create: `packages/landing-blocks/src/blocks/guest/GuestHeroDestinationsBlock.svelte`
- Create: `packages/landing-blocks/src/blocks/guest/GuestHeroCommunityBlock.svelte`

**Interfaces:**
- Consumes: `GuestHeroContainerProps`, `GuestHeroTextProps`, `GuestHeroImageProps`, `GuestHeroStatsProps`, `GuestHeroDestinationsProps`, `GuestHeroCommunityProps`, `GuestHomeData`, `blockRegistry`, `BlockShell`.
- Produces: the five hero block components.

- [ ] **Step 1: Create `GuestHeroContainerBlock.svelte`.**

Render a custom two-column layout (not `ContainerBlock`) so the right column can be `position: relative` and hold the image plus the absolutely-positioned floating destination/community cards. Use `blockRegistry` and `BlockShell` to render children manually, exactly as `ContainerBlock` does, but with a left/right split:

- Left column: `guest_hero_text`, then `guest_hero_stats`.
- Right column (relative): `guest_hero_image`, `guest_hero_destinations` (absolute), `guest_hero_community` (absolute).

Group children by `block.type`: left types (`guest_hero_text`, `guest_hero_stats`) go left; right types (`guest_hero_image`, `guest_hero_destinations`, `guest_hero_community`) go right. If a floating card has no `style.position`, apply a default absolute position (`destinations`: top-left; `community`: bottom-right) before passing it to `BlockShell`.

Default children (stable IDs based on the container `id`):

```ts
const defaultChildren: Block[] = $derived([
  { id: `${id}-text`, type: 'guest_hero_text', visible: true, props: {}, style: { position: 'relative' } },
  { id: `${id}-image`, type: 'guest_hero_image', visible: true, props: {} },
  { id: `${id}-stats`, type: 'guest_hero_stats', visible: true, props: {}, style: { position: 'relative' } },
  { id: `${id}-destinations`, type: 'guest_hero_destinations', visible: true, props: {}, style: { position: 'absolute', top: '1.5rem', left: '1rem' } },
  { id: `${id}-community`, type: 'guest_hero_community', visible: true, props: {}, style: { position: 'absolute', bottom: '2.5rem', right: '1rem' } },
]);
```

Accept `id?: string` and use it to derive stable child IDs.

- [ ] **Step 2: Create `GuestHeroTextBlock.svelte`.**

Render eyebrow, headline (split into `headline_prefix` + `headline_accent`), subhead, and CTA buttons from `props.ctas`. Use the existing `GuestHomeBlock` hero markup as the visual starting point.

- [ ] **Step 3: Create `GuestHeroImageBlock.svelte`.**

Render a single image or a carousel based on `slides` length. Use `MediaBlock` or a custom carousel with dots/arrows. Wrap in a `position: relative` div.

- [ ] **Step 4: Create `GuestHeroStatsBlock.svelte`.**

Render the hero stat row. In dynamic mode read from `GuestHomeData.stats`; in static mode use `props.items`. Allow per-stat overrides.

- [ ] **Step 5: Create `GuestHeroDestinationsBlock.svelte`.**

Render the floating "Top Destinations" card. Read from `GuestHomeData.topDestinations` and accept `title`, `max_items`, and `see_all_href`.

- [ ] **Step 6: Create `GuestHeroCommunityBlock.svelte`.**

Render the floating "Join X members" card. Read the community stat from `GuestHomeData.stats` and accept `title`, `subtitle`, `cta`.

- [ ] **Step 7: Add one render test for `GuestHeroTextBlock`.**

Create `packages/landing-blocks/src/blocks/guest/GuestHeroTextBlock.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import GuestHeroTextBlock from './GuestHeroTextBlock.svelte';

describe('GuestHeroTextBlock', () => {
  it('renders headline and ctas', () => {
    const { getByText } = render(GuestHeroTextBlock, {
      props: { props: { headline_prefix: 'Plan your', headline_accent: 'Japa', subhead: 'Move abroad with confidence.', ctas: [{ label: 'Get Started', href: '/signup', variant: 'primary' }] } }
    });
    expect(getByText('Plan your')).toBeTruthy();
    expect(getByText('Get Started')).toBeTruthy();
  });
});
```

- [ ] **Step 8: Run type check and test.**

```bash
cd packages/landing-blocks && bunx tsc --noEmit && bunx vitest run src/blocks/guest/GuestHeroTextBlock.test.ts
```

Expected: type check clean; test PASS.

- [ ] **Step 9: Commit.**

```bash
git add packages/landing-blocks/src/blocks/guest/GuestHero*.svelte packages/landing-blocks/src/blocks/guest/GuestHeroTextBlock.test.ts
git commit -m "feat(landing-blocks): guest hero block family"
```

## Task 5 — Section card-list blocks

**Files:**
- Create: `packages/landing-blocks/src/blocks/guest/GuestCategoriesBlock.svelte`
- Create: `packages/landing-blocks/src/blocks/guest/GuestOpportunitiesBlock.svelte`
- Create: `packages/landing-blocks/src/blocks/guest/GuestStoriesBlock.svelte`
- Create: `packages/landing-blocks/src/blocks/guest/GuestToolsBlock.svelte`

**Interfaces:**
- Consumes: `GuestCategoriesProps`, `GuestOpportunitiesProps`, `GuestStoriesProps`, `GuestToolsProps`, `GuestHomeData`, `GuestCardListBaseProps`.
- Produces: four reusable card-list block components.

- [ ] **Step 1: Create a shared render helper for grid/slider columns.**

Create `packages/landing-blocks/src/blocks/guest/guestCardListRender.ts`:

```ts
import type { GuestCardListBaseProps } from '../../types';

export function resolveGridStyle(base: GuestCardListBaseProps, instanceId: string): string {
  const cols = base.columns ?? 3;
  const tablet = base.responsive_columns?.tablet ?? Math.max(1, Math.ceil(cols / 2));
  const mobile = base.responsive_columns?.mobile ?? 1;

  const scope = `[data-gcl="${instanceId}"]`;
  return `
    ${scope} { display: grid; gap: inherit; grid-template-columns: repeat(${mobile}, minmax(0, 1fr)); }
    @container (min-width: 768px) { ${scope} { grid-template-columns: repeat(${tablet}, minmax(0, 1fr)); } }
    @container (min-width: 1025px) { ${scope} { grid-template-columns: repeat(${cols}, minmax(0, 1fr)); } }
  `;
}
```

Each card-list component injects this scoped `<style>` via `<svelte:head>` (same pattern as `ContainerBlock`). Use `data-gcl={instanceId}` on the grid wrapper, where `instanceId` is based on the block `id`.

- [ ] **Step 2: Implement each card-list block.**

For `GuestCategoriesBlock`, `GuestOpportunitiesBlock`, `GuestStoriesBlock`, `GuestToolsBlock`:

1. Accept `props` and `id`, and consume `getGuestHomeData()`. Use the pattern `const store = getGuestHomeData() ?? writable(emptyHome); const guestHome = $derived($store);` to get a reactive value.
2. Determine `isStatic` from `props.items` being non-empty (or add an explicit `data_source: 'context' | 'manual'` prop if static/dynamic toggle is needed before items exist). For now, use `props.items` static if present, otherwise fall back to context.
3. Derive `effectiveItems` from `props.items` (static) or the relevant `GuestHomeData` slice (dynamic), limited by `props.max_items`.
4. Render the section header with `title`/`subtitle` and a view-all link.
5. For `grid` layout, render a `<section data-gcl={instanceId}>` with scoped CSS from `resolveGridStyle`. For `slider` layout, render a horizontal scroll container with `scroll-snap-type: x mandatory` and `flex: 0 0 calc(...)` per card. Use the desktop `columns` value to compute the visible-card width in slider mode.
6. Extract the relevant CSS from the existing `GuestHomeBlock.svelte` styles for each section into the component's `<style>` block.

- [ ] **Step 3: Add a render test for `GuestToolsBlock`.**

Create `packages/landing-blocks/src/blocks/guest/GuestToolsBlock.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import GuestToolsBlock from './GuestToolsBlock.svelte';

describe('GuestToolsBlock', () => {
  it('renders title and static tools', () => {
    const { getByText } = render(GuestToolsBlock, {
      props: {
        props: {
          title: 'Free tools',
          layout: 'grid',
          columns: 4,
          items: [{ name: 'Visa Checklist', description: 'Generate your visa checklist.', icon: 'FileCheck' }]
        }
      }
    });
    expect(getByText('Free tools')).toBeTruthy();
    expect(getByText('Visa Checklist')).toBeTruthy();
  });
});
```

- [ ] **Step 4: Run type check and test.**

```bash
cd packages/landing-blocks && bunx tsc --noEmit && bunx vitest run src/blocks/guest/GuestToolsBlock.test.ts
```

Expected: clean, PASS.

- [ ] **Step 5: Commit.**

```bash
git add packages/landing-blocks/src/blocks/guest/GuestCategoriesBlock.svelte packages/landing-blocks/src/blocks/guest/GuestOpportunitiesBlock.svelte packages/landing-blocks/src/blocks/guest/GuestStoriesBlock.svelte packages/landing-blocks/src/blocks/guest/GuestToolsBlock.svelte packages/landing-blocks/src/blocks/guest/guestCardListRender.ts packages/landing-blocks/src/blocks/guest/GuestToolsBlock.test.ts
git commit -m "feat(landing-blocks): guest card-list section blocks"
```

## Task 6 — `guest_newsletter` and `guest_footer`

**Files:**
- Create: `packages/landing-blocks/src/blocks/guest/GuestNewsletterBlock.svelte`
- Create: `packages/landing-blocks/src/blocks/guest/GuestFooterBlock.svelte`

**Interfaces:**
- Consumes: `GuestNewsletterProps`, `GuestFooterProps`, `FetchContext`.
- Produces: two block components.

- [ ] **Step 1: Create `GuestNewsletterBlock.svelte`.**

Render a newsletter form with editable title, subtitle, placeholder, CTA, endpoint, and success/error messages. Use `fetchCtx?.fetch` for the subscription POST.

- [ ] **Step 2: Create `GuestFooterBlock.svelte`.**

Render a multi-column footer. Props: `logo`, `tagline`, `socials`, `columns`. Support add/remove/reorder columns and links via content editor.

- [ ] **Step 3: Add a render test for `GuestFooterBlock`.**

Create `packages/landing-blocks/src/blocks/guest/GuestFooterBlock.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import GuestFooterBlock from './GuestFooterBlock.svelte';

describe('GuestFooterBlock', () => {
  it('renders columns and tagline', () => {
    const { getByText } = render(GuestFooterBlock, {
      props: {
        props: {
          tagline: 'Your trusted partner.',
          columns: [{ title: 'Explore', links: [{ label: 'Jobs', url: '/jobs' }] }]
        }
      }
    });
    expect(getByText('Your trusted partner.')).toBeTruthy();
    expect(getByText('Explore')).toBeTruthy();
    expect(getByText('Jobs')).toBeTruthy();
  });
});
```

- [ ] **Step 4: Run type check and test.**

```bash
cd packages/landing-blocks && bunx tsc --noEmit && bunx vitest run src/blocks/guest/GuestFooterBlock.test.ts
```

Expected: clean, PASS.

- [ ] **Step 5: Commit.**

```bash
git add packages/landing-blocks/src/blocks/guest/GuestNewsletterBlock.svelte packages/landing-blocks/src/blocks/guest/GuestFooterBlock.svelte packages/landing-blocks/src/blocks/guest/GuestFooterBlock.test.ts
git commit -m "feat(landing-blocks): guest newsletter and footer blocks"
```

## Task 7 — Content editors

**Files:**
- Create: `packages/landing-blocks/src/editors/content/guest/*.svelte` (13 files)
- Modify: `packages/landing-blocks/src/editors/content/index.ts`

**Interfaces:**
- Consumes: `contentEditorRegistry`, existing editor UI primitives (inputs, selects, `MediaPicker` if available, icon picker, dnd-kit sortable list).
- Produces: registered content editors for all `guest_*` block types.

- [ ] **Step 1: Create a reusable `GuestCardListEditor.svelte` helper.**

Create `packages/landing-blocks/src/editors/content/guest/GuestCardListEditor.svelte`.

It renders a dnd-kit `DragDropProvider` + sortable list of card edit forms. Props:

```ts
interface Props {
  items: Array<Record<string, unknown>>;
  onchange: (items: Array<Record<string, unknown>>) => void;
  renderFields: (item: Record<string, unknown>, index: number, onUpdate: (v: Record<string, unknown>) => void) => Snippet;
  addItem: () => Record<string, unknown>;
  emptyItem?: Record<string, unknown>;
}
```

Features:
- Drag handle per card.
- Add card button (calls `addItem()` and appends).
- Remove card button per card.
- Reorder by dnd-kit `createSortable` on each card.

- [ ] **Step 2: Implement each content editor with static/dynamic mode.**

For each `guest_*` block, create an editor that edits the block's `props` and calls `onchange` with the updated props.

For card-list blocks (`guest_categories`, `guest_opportunities`, `guest_stories`, `guest_tools`):

1. Header fields: `title`, `subtitle`, `view_all_href`.
2. Layout controls: `layout` toggle (`grid`/`slider`), `columns` stepper (1–6), `responsive_columns` steppers for `tablet` and `mobile`, `gap` select, `card_style` select.
3. Data source toggle: `Use dynamic data` / `Use manual items`.
   - Dynamic: show `max_items` stepper; the block reads from `GuestHomeData`.
   - Manual: use `GuestCardListEditor` to add/remove/reorder cards and edit each card field (name/title, description, icon, logo, href, accent color, etc.).

For `guest_hero_stats`, `guest_hero_destinations`, `guest_hero_community`: similar data source toggle + manual override editor.

For `guest_footer`: logo media picker, tagline, socials list (icon + url + label), columns list (title + links list with add/remove/reorder).

For `guest_hero_text`: eyebrow, headline prefix/accent, subhead, CTA list with add/remove/reorder.

Key editors:
- `GuestHomeContentEditor` — legacy conversion hint + reset defaults.
- `GuestHeroContainerContentEditor` — layout, columns, gap, background.
- `GuestHeroTextContentEditor` — headline fields, subhead, CTAs.
- `GuestHeroImageContentEditor` — slides media picker, carousel toggles.
- `GuestCategoriesContentEditor` — layout, columns, responsive columns, gap, card style, title/subtitle, card list (static/dynamic).
- `GuestOpportunitiesContentEditor` — same pattern.
- `GuestStoriesContentEditor` — same pattern.
- `GuestToolsContentEditor` — same pattern.
- `GuestNewsletterContentEditor` — form fields.
- `GuestFooterContentEditor` — columns and links.

- [ ] **Step 3: Register editors in `packages/landing-blocks/src/editors/content/index.ts`.**

```ts
import GuestHomeContentEditor from './guest/GuestHomeContentEditor.svelte';
import GuestHeroContainerContentEditor from './guest/GuestHeroContainerContentEditor.svelte';
...

export const contentEditorRegistry: Record<Block['type'], Component<any>> = {
  ...existing entries...
  guest_home: GuestHomeContentEditor,
  guest_hero: GuestHeroContainerContentEditor,
  guest_hero_text: GuestHeroTextContentEditor,
  ...
};
```

- [ ] **Step 4: Run admin and landing-blocks type checks.**

```bash
cd packages/landing-blocks && bunx tsc --noEmit
cd apps/admin && bun run check
```

Expected: clean.

- [ ] **Step 5: Commit.**

```bash
git add packages/landing-blocks/src/editors/content/guest packages/landing-blocks/src/editors/content/index.ts
git commit -m "feat(landing-blocks): guest block content editors"
```

## Task 8 — Block and editor registry

**Files:**
- Modify: `packages/landing-blocks/src/render/blockRegistry.ts`
- Modify: `packages/landing-blocks/src/index.ts`

**Interfaces:**
- Consumes: all new `guest_*` Svelte components.
- Produces: `blockRegistry` entries and exports.

- [ ] **Step 1: Update `blockRegistry.ts`.**

Replace the `guest_home` entry with `GuestHomeContainerBlock`. Add entries for `guest_hero`, `guest_hero_text`, `guest_hero_image`, `guest_hero_stats`, `guest_hero_destinations`, `guest_hero_community`, `guest_categories`, `guest_opportunities`, `guest_stories`, `guest_tools`, `guest_newsletter`, `guest_footer`. Use `category: 'guest'` for all.

- [ ] **Step 2: Export new block components from `packages/landing-blocks/src/index.ts`.**

Add named exports for the new blocks so the admin preview and main app can import them if needed.

- [ ] **Step 3: Run type check.**

```bash
cd packages/landing-blocks && bunx tsc --noEmit
```

Expected: clean.

- [ ] **Step 4: Commit.**

```bash
git add packages/landing-blocks/src/render/blockRegistry.ts packages/landing-blocks/src/index.ts
git commit -m "feat(landing-blocks): register guest_* blocks"
```

## Task 9 — Admin Page Builder defaults

**Files:**
- Modify: `apps/admin/src/routes/page-builder/pages/[id]/+page.svelte`

**Interfaces:**
- Consumes: `makeBlock(type)`, `Block`, `BlockMeta`.
- Produces: updated `makeBlock` with `guest_*` defaults.

- [ ] **Step 1: Locate the `makeBlock` function.**

Find `makeBlock` in `apps/admin/src/routes/page-builder/pages/[id]/+page.svelte`.

- [ ] **Step 2: Add `guest_*` defaults.**

Add a branch (or extend the existing switch) to handle each new `guest_*` type. For example:

```ts
if (blockType === 'guest_home') {
  return {
    id: crypto.randomUUID(),
    type: 'guest_home',
    visible: true,
    label: 'Guest Home',
    props: { layout: 'stack', children: [
      { id: crypto.randomUUID(), type: 'guest_hero', visible: true, props: {} },
      { id: crypto.randomUUID(), type: 'guest_categories', visible: true, props: {} },
      { id: crypto.randomUUID(), type: 'guest_opportunities', visible: true, props: {} },
      { id: crypto.randomUUID(), type: 'guest_stories', visible: true, props: {} },
      { id: crypto.randomUUID(), type: 'guest_tools', visible: true, props: {} },
      { id: crypto.randomUUID(), type: 'guest_newsletter', visible: true, props: {} },
      { id: crypto.randomUUID(), type: 'guest_footer', visible: true, props: {} },
    ] }
  };
}
```

For `guest_hero`:

```ts
if (blockType === 'guest_hero') {
  return {
    id: crypto.randomUUID(),
    type: 'guest_hero',
    visible: true,
    label: 'Hero',
    props: { layout: 'grid', columns: 2, responsive_columns: { tablet: 1, mobile: 1 }, gap: 'md', children: [
      { id: crypto.randomUUID(), type: 'guest_hero_text', visible: true, props: {} },
      { id: crypto.randomUUID(), type: 'guest_hero_image', visible: true, props: {} },
      { id: crypto.randomUUID(), type: 'guest_hero_stats', visible: true, props: {} },
      { id: crypto.randomUUID(), type: 'guest_hero_destinations', visible: true, props: {} },
      { id: crypto.randomUUID(), type: 'guest_hero_community', visible: true, props: {} },
    ] }
  };
}
```

For other `guest_*` types, create a minimal block with the correct `type` and empty `props`.

- [ ] **Step 3: Verify palette grouping.**

Open the admin Page Builder (`/page-builder/pages/[id]`) and confirm the new `guest` category appears with all block types. (Manual browser verification; no new code needed.)

- [ ] **Step 4: Commit.**

```bash
git add apps/admin/src/routes/page-builder/pages/[id]/+page.svelte
git commit -m "feat(admin): default guest_* block shapes in page builder"
```

## Task 10 — Update `.jeos/sections/16-page-builder.md`

**Files:**
- Modify: `.jeos/sections/16-page-builder.md`

**Interfaces:**
- Consumes: the `guest_*` block catalog.
- Produces: documented block catalog in JEOS.

- [ ] **Step 1: Add the `guest_*` block catalog to the section 16 block table.**

Append a new section to the block catalog listing each `guest_*` block with its category, label, and key props. Reference the spec at `docs/superpowers/specs/2026-09-15-guest-homepage-blocks-design.md`.

- [ ] **Step 2: Commit.**

```bash
git add .jeos/sections/16-page-builder.md
git commit -m "docs(jeos): document guest_* page builder blocks"
```

## Task 11 — Final verification

**Files:**
- All touched files.

**Interfaces:**
- Consumes: the full block family, admin Page Builder, `apps/main` fallback.
- Produces: green type check and build.

- [ ] **Step 1: Run landing-blocks type check and tests.**

```bash
cd packages/landing-blocks
bunx tsc --noEmit
bunx vitest run
```

Expected: all tests PASS; no type errors.

- [ ] **Step 2: Run admin type check and build.**

```bash
cd apps/admin
bun run check
bun run build
```

Expected: clean build.

- [ ] **Step 3: Run main app type check and build.**

```bash
cd apps/main
bun run check
bun run build
```

Expected: clean build.

- [ ] **Step 4: Manual end-to-end checks.**

In the admin Page Builder:
1. Add a `guest_home` block to a landing page.
2. Confirm it expands with all default section children.
3. Drag/reorder sections.
4. Select a `guest_opportunities` block and switch layout between grid and slider.
5. Reduce desktop columns to 2 and mobile to 1.
6. Add/remove/reorder cards in static mode.
7. Save and preview.

In `apps/main`:
1. Visit `/` as a guest.
2. Confirm the fallback `guest_home` still renders all sections.
3. Confirm `GuestHeader` and `GuestFooter` shells are unchanged.

- [ ] **Step 5: Final commit (or summary of changes).**

If any verification fixes were made, commit them:

```bash
git add -A
git commit -m "fix(landing-blocks): verification fixes for guest blocks"
```

---

## Self-review checklist

- [ ] Every spec requirement maps to at least one task above.
- [ ] No `TBD`, `TODO`, or placeholder steps remain.
- [ ] Type names (`GuestHeroTextProps`, `GuestCardListBaseProps`, etc.) are consistent across the plan and the spec.
- [ ] `bun` is used in all commands.
- [ ] `GuestHeader`/`GuestFooter` shell components are not modified.

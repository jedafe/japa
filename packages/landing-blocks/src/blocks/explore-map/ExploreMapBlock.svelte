<script lang="ts">
  // Ref: japaconnect-engineering/sections/16-page-builder.md (explore_map)
  // World Explorer parent — owns all config + shared interactive state via
  // ExploreMapContext, runs the data-loading effects, and lays out its
  // explore_map_* children into the aside/main regions by type (the same
  // partition pattern as guest_hero's rightTypes). Empty `props.children`
  // renders a seeded default set reproducing the standard layout.
  import type { Block, ExploreMapProps } from '../../types';
  import type { FetchContext } from '../../data/types';
  import {
    ExploreMapContext, setExploreMapCtx, partitionExploreMapChildren,
    defaultExploreMapChildren, exploreMapChildVisible,
    type GeoSpaceLite, type FxCurrency,
  } from './context.svelte';
  import { REGION_BOUNDS } from '../../geo/worldMapData';
  // Resolve children via the lazy registry indirection — a direct
  // `blockRegistry` import would close a circular import (registry → this
  // component → registry) that crashes whenever this module is evaluated
  // before the registry (WikiExploreMap imports this block directly).
  import { blockComponentFor } from '../../render/blockComponent';
  import { isBlockVisible, isBlockEmpty } from '../../render/blockVisibility';
  import { responsiveVisibilityCss } from '../../shell/styleToCss';
  import ScopedCss from '../../shell/ScopedCss.svelte';
  import BlockShell from '../../shell/BlockShell.svelte';

  interface Props {
    props: ExploreMapProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }
  let { props, fetchCtx, geoCountry, id }: Props = $props();

  const ctx = new ExploreMapContext({
    props: () => props,
    fetchCtx: () => fetchCtx,
    geoCountry: () => geoCountry,
  });
  setExploreMapCtx(ctx);

  // Initialize selections from props once (don't clobber on later edits).
  $effect(() => {
    if (!ctx.selView) ctx.selView = ctx.P.views?.[0]?.key ?? '';
    if (!ctx.currency) ctx.currency = ctx.P.default_currency ?? 'USD';
    // initial_bounds takes precedence over initial_region; vbFit ensures the
    // fit runs once even though this effect re-fires when P changes.
    if (!ctx.selRegion && !ctx.vbFit && ctx.P.initial_bounds) {
      ctx.vbFit = true;
      ctx.fitBounds(ctx.P.initial_bounds);
    } else if (!ctx.selRegion && !ctx.vbFit && ctx.P.initial_region && REGION_BOUNDS[ctx.P.initial_region]) {
      ctx.vbFit = true;
      ctx.selectRegion(ctx.P.initial_region);
    }
  });

  // ── Data loading ────────────────────────────────────────────
  // Mount fetch: countries, regions, stats. Reads no block props so it only
  // re-runs if fetchCtx changes — not on every inspector keystroke.
  $effect(() => {
    ctx.loading = true;
    void (async () => {
      const [countries, regions] = await Promise.all([
        ctx.api<{ data: GeoSpaceLite[] }>('/geo/countries'),
        ctx.api<{ data: string[] }>('/spaces/regions'),
      ]);
      ctx.apiCountries = (countries?.data ?? []).filter((c) => c.geo_code);
      ctx.apiRegions = regions?.data ?? [];

      // Stat totals — independent best-effort fetches.
      const counts: Record<string, number> = { countries: ctx.apiCountries.length };
      const [col, blog, svc] = await Promise.all([
        ctx.api<{ data: { total: number } }>('/calculator/cost-of-living/cities?page_size=1'),
        ctx.api<{ meta: { total: number } }>('/blog?page_size=1'),
        ctx.api<{ meta: { total: number } }>('/services?limit=1'),
      ]);
      if (col?.data?.total != null) counts.col_cities = col.data.total;
      if (blog?.meta?.total != null) counts.blog = blog.meta.total;
      if (svc?.meta?.total != null) counts.services = svc.meta.total;
      ctx.statCounts = counts;
      ctx.loading = false;
    })();
  });

  // FX data — keyed only on the configured base currency.
  $effect(() => {
    const base = ctx.P.default_currency ?? 'USD';
    void (async () => {
      const [fxCur, fx] = await Promise.all([
        ctx.api<{ data: FxCurrency[] }>('/fx/currencies'),
        ctx.api<{ data: { rates: Record<string, number> } }>(`/fx/rates?base=${encodeURIComponent(base)}`),
      ]);
      ctx.currencies = fxCur?.data ?? [];
      ctx.fxRates = fx?.data?.rates ?? {};
    })();
  });

  // View-specific pin sets (universities / COL cities), fetched on demand.
  $effect(() => {
    const source = ctx.viewSource(ctx.selView);
    if (!source || source === 'countries') { ctx.viewPins = []; return; }
    void (async () => {
      ctx.viewPins = source === 'universities' ? await ctx.loadUniversityPins() : await ctx.loadColCityPins();
    })();
  });

  // ── Children: seeded defaults + aside/main partition by type ──
  const children = $derived(props.children?.length ? props.children : defaultExploreMapChildren(id ?? 'explore-map'));
  const bottom = $derived(ctx.P.aside_position === 'bottom');
  const parts = $derived(partitionExploreMapChildren(children, bottom));
  // Children self-gate on show_* via exploreMapChildVisible — filtering here
  // keeps gated-off children from mounting an empty BlockShell (and keeps the
  // aside wrapper absent when nothing inside would render, as before).
  const visibleAside = $derived(parts.aside.filter((b) => isBlockVisible(b, geoCountry) && exploreMapChildVisible(ctx.P, b.type)));
  const visibleMain = $derived(parts.main.filter((b) => isBlockVisible(b, geoCountry) && exploreMapChildVisible(ctx.P, b.type)));
  // The toolbar sits inside the header row beside the headline; everything
  // else in the main column renders below it.
  const mainToolbar = $derived(visibleMain.filter((b) => b.type === 'explore_map_toolbar'));
  const mainBody = $derived(visibleMain.filter((b) => b.type !== 'explore_map_toolbar'));

  // Per-device visibility rules for children (scopeKey = block.id).
  const visibilityCss = $derived(responsiveVisibilityCss(children));
</script>

<ScopedCss css={visibilityCss} />

<section class="explore-map {ctx.P.cover_mode ? '' : 'px-4 py-10 sm:px-6'}" id={id}>
  <div class="mx-auto max-w-7xl">
    <div class="flex flex-col gap-5 {bottom ? '' : 'lg:flex-row'}">
      <!-- ══ Panel — beside the map ('left') or stacked below ('bottom') ══ -->
      {#if visibleAside.length > 0}
        <aside class="order-2 w-full shrink-0 {bottom ? 'grid gap-4 sm:grid-cols-2 lg:grid-cols-3' : 'flex flex-col gap-4 lg:order-1 lg:w-60'}">
          {#each visibleAside as block (block.id)}
            {@render childShell(block)}
          {/each}
        </aside>
      {/if}

      <!-- ══ Main column (map first on mobile/tablet) ════════ -->
      <div class="order-1 min-w-0 flex-1 {bottom ? '' : 'lg:order-2'}">
        <div class="mb-4 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div class="min-w-0">
            {#if ctx.P.headline}<h2 class="text-2xl font-bold tracking-tight text-[var(--color-text)] sm:text-[28px]">{ctx.P.headline}</h2>{/if}
            {#if ctx.P.subtitle}<p class="mt-1 text-sm text-[var(--color-muted)]">{ctx.P.subtitle}</p>{/if}
          </div>
          {#each mainToolbar as block (block.id)}
            {@render childShell(block)}
          {/each}
        </div>
        {#each mainBody as block (block.id)}
          {@render childShell(block)}
        {/each}
      </div>
    </div>
  </div>
</section>

{#snippet childShell(block: Block)}
  {@const meta = blockComponentFor(block.type)}
  {#if meta}
    {@const Component = meta.component}
    <BlockShell style={block.style} motion={block.motion} background={block.style?.background} hideIfEmpty={block.visibility_rules?.hide_if_empty} initialHasContent={!isBlockEmpty(block)} scopeKey={block.id}>
      {#if block.type === 'custom'}
        <Component props={block.props} blockDefId={block.block_def_id} {fetchCtx} {geoCountry} id={block.id} />
      {:else}
        <Component props={block.props} {fetchCtx} {geoCountry} id={block.id} />
      {/if}
    </BlockShell>
  {/if}
{/snippet}

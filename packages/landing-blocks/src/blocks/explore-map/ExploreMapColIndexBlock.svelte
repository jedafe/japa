<script lang="ts">
  // Ref: japaconnect-engineering/sections/16-page-builder.md (explore_map)
  // World Explorer child — the cost-of-living index legend. Two variants:
  // a horizontal bar under the map (left-aside mode) and a vertical card in
  // the bottom aside grid (bottom mode), picked from ctx.P.aside_position.
  import type { ExploreMapColIndexProps } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { getExploreMapCtx, exploreMapChildVisible } from './context.svelte';

  interface Props {
    props: ExploreMapColIndexProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }
  let { props }: Props = $props();

  const ctx = getExploreMapCtx();
</script>

{#if ctx && exploreMapChildVisible(ctx.P, 'explore_map_col_index')}
  {#if ctx.P.aside_position === 'bottom'}
    <!-- Card variant — sits in the bottom aside grid -->
    <div class="rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-4 shadow-sm">
      <h3 class="mb-3 text-xs font-semibold tracking-wide text-[var(--color-muted)]">Cost of living index (Monthly)</h3>
      <ul class="space-y-2.5">
        {#each ctx.P.col_bands ?? [] as band (band.key)}
          <li class="flex items-center gap-2.5 rounded-[var(--radius)] px-1.5 py-1 transition-colors hover:bg-[var(--color-surface-subtle)]">
            <span class="h-2.5 w-2.5 shrink-0 rounded-full" style="background:{band.color}"></span>
            <span class="text-xs font-semibold text-[var(--color-text)]">{ctx.bandRange(band.min, band.max)}</span>
            <span class="truncate text-xs text-[var(--color-muted)]">{band.label}</span>
          </li>
        {/each}
      </ul>
    </div>
  {:else}
    <!-- Bar variant — under the map in left-aside mode -->
    <div class="mt-4 rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] px-5 py-4 shadow-sm">
      <h3 class="mb-3 text-xs font-semibold tracking-wide text-[var(--color-muted)]">Cost of living index (Monthly)</h3>
      <ul class="flex flex-wrap items-center gap-x-8 gap-y-2">
        {#each ctx.P.col_bands ?? [] as band (band.key)}
          <li class="flex items-center gap-2">
            <span class="h-2.5 w-2.5 rounded-full" style="background:{band.color}"></span>
            <span class="text-xs font-semibold text-[var(--color-text)]">{ctx.bandRange(band.min, band.max)}</span>
            <span class="text-xs text-[var(--color-muted)]">{band.label}</span>
          </li>
        {/each}
      </ul>
    </div>
  {/if}
{/if}

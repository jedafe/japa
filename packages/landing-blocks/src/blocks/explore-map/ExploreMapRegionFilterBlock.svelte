<script lang="ts">
  // Ref: japaconnect-engineering/sections/16-page-builder.md (explore_map)
  // World Explorer child — the region filter card. Selecting a region zooms
  // the canvas and dims out-of-region countries via ctx.selectRegion.
  import type { ExploreMapRegionFilterProps } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { getExploreMapCtx, exploreMapChildVisible } from './context.svelte';
  import * as LucideIcons from 'lucide-svelte';

  interface Props {
    props: ExploreMapRegionFilterProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }
  let { props }: Props = $props();

  const ctx = getExploreMapCtx();
</script>

{#if ctx && exploreMapChildVisible(ctx, 'explore_map_region_filter')}
  <div class="rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-4 shadow-sm">
    <h3 class="mb-2.5 text-xs font-semibold tracking-wide text-[var(--color-muted)]">Region filter</h3>
    <ul class="space-y-1">
      <li>
        <button
          type="button"
          class="flex w-full items-center gap-2 rounded-[var(--radius)] px-2 py-1.5 text-sm transition-colors {ctx.selRegion === '' ? 'bg-[var(--color-primary)]/10 font-medium text-[var(--color-primary)]' : 'text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'}"
          onclick={() => ctx.resetView()}
        >
          <LucideIcons.Globe size={14} /> All Regions
        </button>
      </li>
      {#each ctx.regionOptions as region (region)}
        <li>
          <button
            type="button"
            class="flex w-full items-center gap-2 rounded-[var(--radius)] px-2 py-1.5 text-sm transition-colors {ctx.selRegion === region ? 'bg-[var(--color-primary)]/10 font-medium text-[var(--color-primary)]' : 'text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'}"
            onclick={() => ctx.selectRegion(region)}
            aria-pressed={ctx.selRegion === region}
          >
            <LucideIcons.MapPin size={14} class="opacity-60" /> {region}
          </button>
        </li>
      {/each}
    </ul>
  </div>
{/if}

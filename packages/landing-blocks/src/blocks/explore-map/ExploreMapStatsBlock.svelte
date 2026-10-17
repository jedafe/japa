<script lang="ts">
  // Ref: japaconnect-engineering/sections/16-page-builder.md (explore_map)
  // World Explorer child — the quick-stats aside card. In bottom-aside mode
  // the canvas floats the same stats over the map instead (the
  // exploreMapChildVisible gate suppresses this card then).
  import type { ExploreMapStatsProps } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { getExploreMapCtx, exploreMapChildVisible } from './context.svelte';

  interface Props {
    props: ExploreMapStatsProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }
  let { props }: Props = $props();

  const ctx = getExploreMapCtx();
</script>

{#if ctx && exploreMapChildVisible(ctx.P, 'explore_map_stats')}
  <!-- Overlay placement: the canvas floats stats on lg+ — this card is the
       mobile-only fallback (aside on 'left', below-map column on 'bottom'). -->
  {@const overlay = ctx.P.aside_position === 'bottom' || ctx.P.stats_position === 'overlay'}
  <div class="rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-4 shadow-sm {overlay ? 'lg:hidden' : ''}">
    <h3 class="mb-3 text-xs font-semibold tracking-wide text-[var(--color-muted)]">Quick stats</h3>
    <ul class="space-y-3">
      {#each ctx.P.stats ?? [] as stat (stat.label)}
        {@const Icon = ctx.iconFor(stat.icon)}
        <li class="flex items-center gap-2.5">
          <span class="grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
            {#if Icon}<Icon size={15} />{/if}
          </span>
          <span class="min-w-0">
            <span class="block text-sm font-bold leading-tight text-[var(--color-text)]">{ctx.statValue(stat.source, stat.value, stat.suffix)}</span>
            <span class="block truncate text-[11px] text-[var(--color-muted)]">{stat.label}</span>
          </span>
        </li>
      {/each}
    </ul>
  </div>
{/if}

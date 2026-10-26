<script lang="ts">
  // Ref: japaconnect-engineering/sections/16-page-builder.md (explore_map)
  // World Explorer child — the category legend card. Toggling a category
  // writes ctx.selCategory, which filters pins on the canvas.
  import type { ExploreMapLegendProps } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { getExploreMapCtx, exploreMapChildVisible } from './context.svelte';
  import LazyIcon from '../../lib/LazyIcon.svelte';

  interface Props {
    props: ExploreMapLegendProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }
  let { props }: Props = $props();

  const ctx = getExploreMapCtx();
</script>

{#if ctx && exploreMapChildVisible(ctx.P, 'explore_map_legend')}
  <div class="rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-4 shadow-sm">
    <p class="mb-3 text-xs font-semibold tracking-wide text-[var(--color-muted)]">Map legend</p>
    <ul class="space-y-2.5">
      {#each ctx.P.categories ?? [] as cat (cat.key)}
        
        <li>
          <button
            type="button"
            class="flex w-full items-center gap-2.5 rounded-[var(--radius)] px-1.5 py-1 text-start transition-colors hover:bg-[var(--color-surface-subtle)] {ctx.selCategory === cat.key ? 'bg-[var(--color-surface-subtle)] ring-1 ring-[var(--color-border)]' : ''}"
            onclick={() => (ctx.selCategory = ctx.selCategory === cat.key ? '' : cat.key)}
            aria-pressed={ctx.selCategory === cat.key}
          >
            <span class="grid h-8 w-8 shrink-0 place-items-center rounded-lg text-white" style="background:{cat.color}">
              <LazyIcon name={cat.icon} size={15} />
            </span>
            <span class="min-w-0">
              <span class="block truncate text-xs font-semibold text-[var(--color-text)]">{cat.label}</span>
              {#if cat.sublabel}<span class="block truncate text-[11px] text-[var(--color-muted)]">{cat.sublabel}</span>{/if}
            </span>
          </button>
        </li>
      {/each}
    </ul>
  </div>
{/if}

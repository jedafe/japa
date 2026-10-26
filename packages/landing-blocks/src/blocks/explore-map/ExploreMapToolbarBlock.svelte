<script lang="ts">
  // Ref: japaconnect-engineering/sections/16-page-builder.md (explore_map)
  // World Explorer child — the View/Show/Currency toolbar. Reads + writes the
  // shared parent context; config lives on the parent explore_map block.
  import type { ExploreMapToolbarProps } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { getExploreMapCtx, exploreMapChildVisible } from './context.svelte';
  import { MapPin, ChevronDown, LayoutGrid, CircleDollarSign } from 'lucide-svelte';

  interface Props {
    props: ExploreMapToolbarProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }
  let { props }: Props = $props();

  const ctx = getExploreMapCtx();
</script>

{#if ctx && exploreMapChildVisible(ctx.P, 'explore_map_toolbar')}
  <div class="flex shrink-0 divide-x divide-[var(--color-border)] overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-sm">
    <label class="flex items-center gap-2 px-3 py-2">
      <MapPin size={15} class="text-[var(--color-primary)]" />
      <span class="flex flex-col">
        <span class="text-[10px] leading-tight text-[var(--color-muted)]">View</span>
        <span class="em-select-wrap"><select
          bind:value={ctx.selView}
          class="em-select"
        >
          {#each ctx.P.views ?? [] as v (v.key)}<option value={v.key}>{v.label}</option>{/each}
        </select><ChevronDown size={11} class="em-chevron" /></span>
      </span>
    </label>
    <label class="flex items-center gap-2 px-3 py-2">
      <LayoutGrid size={15} class="text-[var(--color-primary)]" />
      <span class="flex flex-col">
        <span class="text-[10px] leading-tight text-[var(--color-muted)]">Show</span>
        <span class="em-select-wrap"><select
          bind:value={ctx.selCategory}
          class="em-select"
        >
          <option value="">All Categories</option>
          {#each ctx.P.categories ?? [] as c (c.key)}<option value={c.key}>{c.label}</option>{/each}
        </select><ChevronDown size={11} class="em-chevron" /></span>
      </span>
    </label>
    {#if ctx.currencies.length > 0}
      <label class="flex items-center gap-2 px-3 py-2">
        <CircleDollarSign size={15} class="text-[var(--color-primary)]" />
        <span class="flex flex-col">
          <span class="text-[10px] leading-tight text-[var(--color-muted)]">Currency</span>
          <span class="em-select-wrap"><select
            bind:value={ctx.currency}
            class="em-select"
          >
            {#each ctx.currencies as c (c.code)}<option value={c.code}>{c.code}</option>{/each}
          </select><ChevronDown size={11} class="em-chevron" /></span>
        </span>
      </label>
    {/if}
  </div>
{/if}

<style>
  .em-select-wrap {
    position: relative;
    display: inline-flex;
    align-items: center;
  }
  .em-select {
    appearance: none;
    -webkit-appearance: none;
    margin: -2px;
    padding: 2px 16px 2px 2px;
    background: transparent;
    border: none;
    color: var(--color-text);
    font-size: 0.75rem;
    font-weight: 600;
    line-height: 1.2;
    cursor: pointer;
    outline: none;
  }
  .em-select option {
    background-color: var(--color-surface);
    color: var(--color-text);
  }
  :global([data-theme="dark"]) .em-select {
    color-scheme: dark;
  }
  .em-select-wrap :global(.em-chevron) {
    position: absolute;
    inset-inline-end: 0;
    pointer-events: none;
    color: var(--color-muted);
  }
</style>

<script lang="ts">
  import type { LiveRateTickerProps } from '../types';
  import type { FetchContext, RemittanceRate } from '../data/types';
  import { fetchRemittanceRates } from '../data/entityFetchers';

  interface Props {
    props: LiveRateTickerProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
  }
  let { props, fetchCtx, geoCountry }: Props = $props();

  let rates = $state<RemittanceRate[]>([]);
  let loading = $state(true);
  let error = $state(false);

  $effect(() => {
    if (!fetchCtx) { loading = false; return; }
    loading = true;
    fetchRemittanceRates(fetchCtx, props.corridors, props.limit)
      .then((result) => {
        // Geo-aware filtering: if geo_aware and geoCountry, filter corridors that include the visitor's country.
        if (props.geo_aware && geoCountry) {
          rates = result.filter(r =>
            r.from_country?.toUpperCase() === geoCountry.toUpperCase() ||
            r.to_country?.toUpperCase() === geoCountry.toUpperCase()
          );
        } else {
          rates = result;
        }
        error = false;
      })
      .catch(() => { error = true; })
      .finally(() => { loading = false; });
  });

  const speed = $derived(props.speed ?? 30);
</script>

<section class="px-6 py-4">
  <div class="mx-auto max-w-6xl overflow-hidden">
    {#if loading}
      <div class="flex gap-3 {props.layout === 'horizontal' ? 'overflow-hidden' : 'flex-col'}">
        {#each Array(props.limit) as _}
          <div class="h-12 w-40 animate-pulse rounded-[var(--radius)] bg-[var(--color-surface-subtle)]"></div>
        {/each}
      </div>
    {:else if error || rates.length === 0}
      <!-- Silent fail — don't show error for rate ticker -->
    {:else if props.layout === 'horizontal'}
      <div class="relative overflow-hidden">
        <div class="flex gap-3 whitespace-nowrap" style={`animation: marquee ${speed}s linear infinite`}>
          {#each [...rates, ...rates] as rate}
            <div class="flex flex-shrink-0 items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2">
              <span class="text-sm font-medium text-[var(--color-text)]">{rate.corridor}</span>
              <span class="text-sm font-semibold text-[var(--color-primary)]">{rate.rate}</span>
              {#if rate.provider}
                <span class="text-xs text-[var(--color-muted)]">{rate.provider}</span>
              {/if}
            </div>
          {/each}
        </div>
      </div>
    {:else}
      <div class="flex flex-col gap-2">
        {#each rates as rate}
          <div class="flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2">
            <span class="text-sm font-medium text-[var(--color-text)]">{rate.corridor}</span>
            <div class="flex items-center gap-3">
              <span class="text-sm font-semibold text-[var(--color-primary)]">{rate.rate}</span>
              {#if rate.provider}
                <span class="text-xs text-[var(--color-muted)]">{rate.provider}</span>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </div>
</section>

<style>
  @keyframes marquee {
    0% { transform: translateX(0); }
    100% { transform: translateX(-50%); }
  }
  :global(.marquee-pause:hover) {
    animation-play-state: paused;
  }
</style>

<script lang="ts">
  import type { GuestHeroStatsProps, GuestHomeData, GuestHomeStat } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { getGuestHomeData } from '../../data/guestHomeContext';

  interface Props {
    props: GuestHeroStatsProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, id = 'guest-hero-stats' }: Props = $props();

  const guestHomeStore = getGuestHomeData();
  let guestHome: GuestHomeData | undefined = $state(undefined);

  $effect(() => {
    if (!guestHomeStore) return;
    const unsub = guestHomeStore.subscribe((v) => (guestHome = v));
    return unsub;
  });

  function getIcon(name: string | undefined): string {
    return name ?? 'Users';
  }

  const items = $derived<GuestHomeStat[]>(
    props.data_source === 'manual' && props.items
      ? props.items
      : (guestHome?.stats ?? [])
  );

  const visibleItems = $derived(items.slice(0, props.max_items ?? 4));
</script>

<!-- Ref: japaconnect-engineering/sections/16-page-builder.md -->
<div class="hero-stats" {id}>
  {#if props.title}
    <h3 class="hero-stats-title">{props.title}</h3>
  {/if}
  <div class="hero-stats-grid">
    {#each visibleItems as stat (stat.label)}
      <div class="hero-stat">
        <span class="hero-stat-value">{stat.value}</span>
        <span class="hero-stat-label">{stat.label}</span>
      </div>
    {/each}
  </div>
</div>

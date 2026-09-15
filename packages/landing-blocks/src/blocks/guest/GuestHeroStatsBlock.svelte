<script lang="ts">
  import type { GuestHeroStatsProps, GuestHomeData, GuestHomeStat } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { getGuestHomeData } from '../../data/guestHomeContext';
  import { writable } from 'svelte/store';
  import { getLucideIcon } from './guestIconMap';

  interface Props {
    props: GuestHeroStatsProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, id = 'guest-hero-stats' }: Props = $props();

  const guestHomeStore = $derived(getGuestHomeData() ?? writable<GuestHomeData | undefined>(undefined));
  const guestHome = $derived($guestHomeStore);

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
<div class="guest-hero-stats" {id}>
  {#if props.title}
    <h3 class="hero-stats-title">{props.title}</h3>
  {/if}
  <div class="hero-stats">
    {#each visibleItems as stat (stat.label)}
      <div class="hero-stat">
        <div class="hero-stat-icon">
          {#if getLucideIcon(stat.icon)}
            {@const Icon = getLucideIcon(stat.icon)}
            <Icon size={20} />
          {/if}
        </div>
        <div class="hero-stat-body">
          <span class="hero-stat-value">{stat.value}</span>
          <span class="hero-stat-label">{stat.label}</span>
        </div>
      </div>
    {/each}
  </div>
</div>

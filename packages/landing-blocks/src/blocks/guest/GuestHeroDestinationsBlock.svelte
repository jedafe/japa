<script lang="ts">
  import type { GuestHeroDestinationsProps, GuestHomeData, GuestHomeDestination } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { Globe, ArrowRight } from 'lucide-svelte';
  import { getGuestHomeData } from '../../data/guestHomeContext';

  interface Props {
    props: GuestHeroDestinationsProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props }: Props = $props();

  const guestHomeStore = getGuestHomeData();
  let guestHome = $state<GuestHomeData | undefined>(undefined);

  $effect(() => {
    if (!guestHomeStore) return;
    const unsub = guestHomeStore.subscribe((v) => (guestHome = v));
    return unsub;
  });

  const items = $derived<GuestHomeDestination[]>(
    props.data_source === 'manual' && props.items
      ? props.items
      : (guestHome?.topDestinations ?? [])
  );

  const visibleItems = $derived(items.slice(0, props.max_items ?? 4));
  const title = $derived(props.title ?? 'Top Destinations');
  const href = $derived(props.see_all_href ?? '/explore');
</script>

<!-- Ref: japaconnect-engineering/sections/16-page-builder.md -->
<div class="floating-card floating-card-destinations">
  <h3 class="floating-card-title">{title}</h3>
  <ul class="destination-list">
    {#each visibleItems as dest (dest.id)}
      <li class="destination-item">
        <div class="destination-flag">
          {#if dest.icon_url}
            <img src={dest.icon_url} alt="" class="destination-flag-img" />
          {:else}
            <Globe size={16} />
          {/if}
        </div>
        <span class="destination-name">{dest.name}</span>
      </li>
    {/each}
  </ul>
  <a href={href} class="destination-all">
    See all countries
    <ArrowRight size={14} />
  </a>
</div>

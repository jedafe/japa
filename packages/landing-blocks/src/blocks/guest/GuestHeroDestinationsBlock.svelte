<script lang="ts">
  import type { GuestHeroDestinationsProps, GuestHomeData, GuestHomeDestination } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { Globe, ArrowRight } from 'lucide-svelte';
  import { getGuestHomeData } from '../../data/guestHomeContext';
  import { writable } from 'svelte/store';

  interface Props {
    props: GuestHeroDestinationsProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props }: Props = $props();

  const guestHomeStore = $derived(getGuestHomeData() ?? writable<GuestHomeData | undefined>(undefined));
  const guestHome = $derived($guestHomeStore);

  const items = $derived<GuestHomeDestination[]>(
    props.data_source === 'manual' && props.items
      ? props.items
      : (guestHome?.topDestinations ?? [])
  );

  const title = $derived(props.title ?? 'Top Destinations');
  const href = $derived(props.see_all_href ?? '/explore');

  function countryCodeToEmoji(code: string | undefined): string | null {
    if (!code || !/^[a-zA-Z]{2}$/.test(code)) return null;
    const points = [...code.toUpperCase()].map((c) => 127397 + c.charCodeAt(0));
    return String.fromCodePoint(...points);
  }

  const visibleItems = $derived(
    items.slice(0, props.max_items ?? 4).map((dest) => ({
      ...dest,
      emoji: countryCodeToEmoji(dest.id)
    }))
  );
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
          {:else if dest.emoji}
            <span class="destination-flag-emoji" aria-hidden="true">{dest.emoji}</span>
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

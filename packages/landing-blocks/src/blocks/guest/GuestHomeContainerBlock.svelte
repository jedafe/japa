<script lang="ts">
  import type { Block, GuestHomeContainerProps, GuestHomeData } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { writable } from 'svelte/store';
  import { getGuestHomeData } from '../../data/guestHomeContext';
  import GuestHomeLegacyBlock from '../GuestHomeLegacyBlock.svelte';
  import ContainerBlock from '../ContainerBlock.svelte';

  interface Props {
    props: GuestHomeContainerProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, fetchCtx, geoCountry, id = 'guest-home' }: Props = $props();

  const emptyHome: GuestHomeData = { topDestinations: [], opportunities: [], stories: [], tools: [], stats: [], categories: [] };
  const guestHomeStore = $derived(getGuestHomeData() ?? writable(emptyHome));
  const guestHome = $derived($guestHomeStore);

  const hasLegacyData = $derived(
    (props.copy && Object.keys(props.copy).length > 0) ||
    !!props.hero_image ||
    (!!props.logo && (props.logo.light || props.logo.dark)) ||
    !!props.newsletter_endpoint
  );

  const isLegacy = $derived(hasLegacyData && (!props.children || props.children.length === 0));

  const defaultChildren: Block[] = $derived([
    { id: `${id}-hero`, type: 'guest_hero', visible: true, props: {} },
    { id: `${id}-categories`, type: 'guest_categories', visible: true, props: {} },
    { id: `${id}-opportunities`, type: 'guest_opportunities', visible: true, props: {} },
    { id: `${id}-stories`, type: 'guest_stories', visible: true, props: {} },
    { id: `${id}-tools`, type: 'guest_tools', visible: true, props: {} },
    { id: `${id}-newsletter`, type: 'guest_newsletter', visible: true, props: {} },
    { id: `${id}-footer`, type: 'guest_footer', visible: true, props: {} },
  ]);

  const effectiveChildren = $derived(props.children && props.children.length > 0 ? props.children : defaultChildren);
</script>

{#if isLegacy}
  <GuestHomeLegacyBlock {props} {guestHome} {fetchCtx} />
{:else}
  <ContainerBlock props={{ layout: 'stack', gap: 'none', children: effectiveChildren }} {fetchCtx} {geoCountry} id={id} />
{/if}

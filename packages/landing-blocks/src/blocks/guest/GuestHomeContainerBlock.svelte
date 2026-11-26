<script lang="ts">
  import type { Block, GuestHomeContainerProps, GuestHomeData } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { writable } from 'svelte/store';
  import { getGuestHomeData } from '../../data/guestHomeContext';
  import { getFeaturesData } from '../../data/featuresContext';
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

  const featuresStore = $derived(getFeaturesData() ?? writable<Record<string, boolean> | undefined>(undefined));

  const defaultChildren: Block[] = $derived.by(() => {
    const blocks: Block[] = [
      { id: `${id}-hero`, type: 'guest_hero', visible: true, props: {} },
      { id: `${id}-categories`, type: 'guest_categories', visible: true, props: {} },
      { id: `${id}-opportunities`, type: 'guest_opportunities', visible: true, props: {} },
      { id: `${id}-stories`, type: 'guest_stories', visible: true, props: {} },
      { id: `${id}-tools`, type: 'guest_tools', visible: true, props: {} },
    ];
    // feature.newsletter kill-switch (§62): omit the signup section when
    // the flag is off so guests never see a form that would 404 on submit.
    if (($featuresStore?.['feature.newsletter'] ?? fetchCtx?.features?.['feature.newsletter']) !== false) {
      blocks.push({ id: `${id}-newsletter`, type: 'guest_newsletter', visible: true, props: {} });
    }
    blocks.push({ id: `${id}-footer`, type: 'guest_footer', visible: true, props: {} });
    return blocks;
  });

  const effectiveChildren = $derived(props.children && props.children.length > 0 ? props.children : defaultChildren);
</script>

{#if isLegacy}
  <GuestHomeLegacyBlock {props} {guestHome} {fetchCtx} />
{:else}
  <ContainerBlock props={{ layout: 'stack', gap: 'none', columns: 1, children: effectiveChildren }} {fetchCtx} {geoCountry} id={id} />
{/if}

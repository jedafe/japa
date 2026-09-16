<script lang="ts">
  import type { GuestHeroDestinationsProps, GuestHomeData, GuestHomeDestination, Block, GuestHeroDestinationItemProps } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { Globe, ArrowRight } from 'lucide-svelte';
  import { getGuestHomeData } from '../../data/guestHomeContext';
  import { writable } from 'svelte/store';
  import { blockRegistry } from '../../render/blockRegistry';
  import { isBlockVisible, isBlockEmpty } from '../../render/blockVisibility';
  import { countryCodeToEmoji } from '../../lib/countryEmoji';
  import BlockShell from '../../shell/BlockShell.svelte';

  interface Props {
    props: GuestHeroDestinationsProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, fetchCtx, geoCountry, id }: Props = $props();

  const guestHomeStore = $derived(getGuestHomeData() ?? writable<GuestHomeData | undefined>(undefined));
  const guestHome = $derived($guestHomeStore);

  const title = $derived(props.title ?? 'Top Destinations');
  const href = $derived(props.see_all_href ?? '/explore');
  // If children exist without an explicit source, assume manual so the seeded
  // blocks actually render. When there are no children, fall back to context.
  const dataSource = $derived(
    props.data_source ?? (props.children && props.children.length > 0 ? 'manual' : 'context')
  );

  // Context data list, capped by max_items.
  const contextItems = $derived<GuestHomeDestination[]>(
    (guestHome?.topDestinations ?? []).slice(0, props.max_items ?? 4)
  );

  // Manual children (data_source === 'manual') are rendered as-is.
  const manualChildren = $derived<Block[] | undefined>(
    dataSource === 'manual' ? props.children : undefined
  );

  // Context template: the first block in children is cloned once per context item,
  // with its props merged from the destination data. If there is no template, fall
  // back to legacy rendering.
  const template = $derived<Block | undefined>(
    dataSource === 'context' && props.children && props.children.length > 0
      ? props.children[0]
      : undefined
  );

  function cloneTemplate(dest: GuestHomeDestination): Block {
    const base = template as Block;
    const itemProps: GuestHeroDestinationItemProps = {
      ...base.props,
      name: dest.name,
      icon_url: dest.icon_url,
      href: dest.href,
      code: dest.id,
    };
    return { ...base, id: `${base.id}-${dest.id}`, props: itemProps } as Block;
  }

  const visibleItems = $derived<Block[] | undefined>(
    dataSource === 'context' && template
      ? contextItems.map(cloneTemplate)
      : undefined
  );

  // Legacy data for non-template, non-manual path.
  const legacyItems = $derived<GuestHomeDestination[] | undefined>(
    manualChildren || visibleItems
      ? undefined
      : dataSource === 'manual' && props.items
        ? props.items.slice(0, props.max_items ?? 4)
        : contextItems
  );
</script>

<!-- Ref: japaconnect-engineering/sections/16-page-builder.md -->
<div class="floating-card floating-card-destinations">
  <h3 class="floating-card-title">{title}</h3>
  <ul class="destination-list">
    {#if manualChildren}
      {#each manualChildren as block (block.id)}
        {#if isBlockVisible(block, geoCountry)}
          {@const meta = blockRegistry[block.type]}
          {#if meta}
            <li class="destination-item" data-item={block.id}>
              <BlockShell style={block.style} motion={block.motion} background={block.style?.background} hideIfEmpty={block.visibility_rules?.hide_if_empty} initialHasContent={!isBlockEmpty(block)}>
                {@const Component = meta.component}
                {#if block.type === 'custom'}
                  <Component props={block.props} blockDefId={block.block_def_id} {fetchCtx} {geoCountry} id={block.id} />
                {:else}
                  <Component props={block.props} {fetchCtx} {geoCountry} id={block.id} />
                {/if}
              </BlockShell>
            </li>
          {/if}
        {/if}
      {/each}
    {:else if visibleItems}
      {#each visibleItems as block (block.id)}
        {#if isBlockVisible(block, geoCountry)}
          {@const meta = blockRegistry[block.type]}
          {#if meta}
            <li class="destination-item" data-item={block.id}>
              <BlockShell style={block.style} motion={block.motion} background={block.style?.background} hideIfEmpty={block.visibility_rules?.hide_if_empty} initialHasContent={!isBlockEmpty(block)}>
                {@const Component = meta.component}
                {#if block.type === 'custom'}
                  <Component props={block.props} blockDefId={block.block_def_id} {fetchCtx} {geoCountry} id={block.id} />
                {:else}
                  <Component props={block.props} {fetchCtx} {geoCountry} id={block.id} />
                {/if}
              </BlockShell>
            </li>
          {/if}
        {/if}
      {/each}
    {:else}
      {#each legacyItems ?? [] as dest (dest.id)}
        <li class="destination-item">
          <div class="destination-flag">
            {#if dest.icon_url}
              <img src={dest.icon_url} alt="" class="destination-flag-img" />
            {:else}
              {@const emoji = countryCodeToEmoji(dest.id)}
              {#if emoji}
                <span class="destination-flag-emoji" aria-hidden="true">{emoji}</span>
              {:else}
                <Globe size={16} />
              {/if}
            {/if}
          </div>
          <span class="destination-name">{dest.name}</span>
        </li>
      {/each}
    {/if}
  </ul>
  <a href={href} class="destination-all">
    See all countries
    <ArrowRight size={14} />
  </a>
</div>

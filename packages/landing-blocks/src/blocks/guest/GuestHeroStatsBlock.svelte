<script lang="ts">
  import type { GuestHeroStatsProps, GuestHomeData, GuestHomeStat, Block, GuestHeroStatItemProps } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { getGuestHomeData } from '../../data/guestHomeContext';
  import { writable } from 'svelte/store';
  import { getLucideIcon } from './guestIconMap';
  import { resolveGuestItems } from './guestChildren';
  import GuestChildren from './GuestChildren.svelte';

  interface Props {
    props: GuestHeroStatsProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, fetchCtx, geoCountry, id = 'guest-hero-stats' }: Props = $props();

  const guestHomeStore = $derived(getGuestHomeData() ?? writable<GuestHomeData | undefined>(undefined));
  const guestHome = $derived($guestHomeStore);

  function getIcon(name: string | undefined): string {
    return name ?? 'Users';
  }

  function cloneForContext(template: Block, stat: GuestHomeStat, index: number): Block {
    const itemProps: GuestHeroStatItemProps = {
      ...(template.props as GuestHeroStatItemProps),
      value: stat.value,
      label: stat.label,
      icon: stat.icon,
    };
    return { ...template, id: `${template.id}-${index}`, props: itemProps } as Block;
  }

  const res = $derived(
    resolveGuestItems<GuestHomeStat>({
      dataSource: props.data_source,
      children: props.children,
      items: props.items,
      contextItems: guestHome?.stats ?? [],
      maxItems: props.max_items ?? 4,
      cloneForContext,
    })
  );

  const childBlocks = $derived(res.manualChildren ?? res.contextChildren);
</script>

<!-- Ref: japaconnect-engineering/sections/16-page-builder.md -->
<div class="guest-hero-stats" {id}>
  {#if props.title}
    <h3 class="hero-stats-title">{props.title}</h3>
  {/if}
  <div class="hero-stats">
    {#if childBlocks}
      <GuestChildren blocks={childBlocks} {fetchCtx} {geoCountry} />
    {:else}
      {#each res.legacyItems ?? [] as stat (stat.label)}
        <div class="hero-stat">
          <div class="hero-stat-icon">
            {#if getLucideIcon(getIcon(stat.icon))}
              {@const Icon = getLucideIcon(getIcon(stat.icon))}
              <Icon size={20} />
            {/if}
          </div>
          <div class="hero-stat-body">
            <span class="hero-stat-value">{stat.value}</span>
            <span class="hero-stat-label">{stat.label}</span>
          </div>
        </div>
      {/each}
    {/if}
  </div>
</div>

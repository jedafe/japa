<script lang="ts">
  import type { GuestToolsProps, GuestHomeData, GuestHomeTool } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { getGuestHomeData } from '../../data/guestHomeContext';
  import GuestCardList from './GuestCardList.svelte';
  import { getLucideIcon } from './guestIconMap';

  interface Props {
    props: GuestToolsProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, id = 'guest-tools' }: Props = $props();

  const guestHomeStore = getGuestHomeData();
  let guestHome = $state<GuestHomeData | undefined>(undefined);

  $effect(() => {
    if (!guestHomeStore) return;
    const unsub = guestHomeStore.subscribe((v) => (guestHome = v));
    return unsub;
  });

  const items = $derived<GuestHomeTool[]>(
    props.data_source === 'manual' && props.items
      ? props.items
      : (guestHome?.tools ?? [])
  );

  const visibleItems = $derived(items.slice(0, props.max_items ?? 8));
  const title = $derived(props.title ?? 'Powerful Tools & Resources');
  const subtitle = $derived(props.subtitle ?? 'Free tools to help you plan, apply, and succeed.');

  function toolHref(tool: GuestHomeTool): string {
    return tool.href ?? (tool.slug ? `/tools/${tool.slug}` : '/');
  }
</script>

<!-- Ref: japaconnect-engineering/sections/16-page-builder.md -->
<GuestCardList
  {title}
  {subtitle}
  view_all_href={props.view_all_href}
  view_all_label="View all tools"
  layout={props.layout ?? 'grid'}
  columns={props.columns ?? 3}
  responsive_columns={props.responsive_columns ?? { tablet: 2, mobile: 1 }}
  gap={props.gap ?? 'lg'}
  card_style={props.card_style ?? 'default'}
  items={visibleItems}
  {id}
>
  {#snippet card(tool: GuestHomeTool)}
    {@const Icon = getLucideIcon(tool.icon)}
    <a href={toolHref(tool)} class="tool-card group">
      <div
        class="tool-icon"
        style="background-color: {tool.accent ? `color-mix(in srgb, ${tool.accent} 15%, transparent)` : 'var(--color-surface-subtle)'}; color: {tool.accent ?? 'var(--color-primary)'}"
      >
        {#if Icon}
          <Icon size={24} />
        {/if}
      </div>
      <h3 class="tool-name">{tool.name}</h3>
      <p class="tool-description">{tool.description}</p>
    </a>
  {/snippet}
</GuestCardList>

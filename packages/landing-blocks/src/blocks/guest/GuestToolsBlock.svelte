<script lang="ts">
  import type { GuestToolsProps, GuestHomeData, GuestHomeTool, Block, GuestToolItemProps } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { getGuestHomeData } from '../../data/guestHomeContext';
  import { writable } from 'svelte/store';
  import GuestCardList from './GuestCardList.svelte';
  import { getLucideIcon } from './guestIconMap';
  import { resolveGuestItems } from './guestChildren';

  interface Props {
    props: GuestToolsProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, fetchCtx, geoCountry, id = 'guest-tools' }: Props = $props();

  const guestHomeStore = $derived(getGuestHomeData() ?? writable<GuestHomeData | undefined>(undefined));
  const guestHome = $derived($guestHomeStore);

  function cloneForContext(template: Block, tool: GuestHomeTool, index: number): Block {
    const itemProps: GuestToolItemProps = {
      ...(template.props as GuestToolItemProps),
      name: tool.name,
      slug: tool.slug,
      description: tool.description,
      href: tool.href,
      icon: tool.icon,
      badge: tool.badge,
      accent: tool.accent,
    };
    return { ...template, id: `${template.id}-${tool.id ?? index}`, props: itemProps } as Block;
  }

  const res = $derived(
    resolveGuestItems<GuestHomeTool>({
      dataSource: props.data_source,
      children: props.children,
      items: props.items,
      contextItems: guestHome?.tools ?? [],
      maxItems: props.max_items ?? 6,
      cloneForContext,
    })
  );

  const childBlocks = $derived(res.manualChildren ?? res.contextChildren);
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
  items={res.legacyItems ?? []}
  blocks={childBlocks}
  {fetchCtx}
  {geoCountry}
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

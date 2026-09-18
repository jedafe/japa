<script lang="ts">
  import type { GuestCategoriesProps, GuestHomeData, GuestHomeCategory, Block, GuestCategoryItemProps } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { getGuestHomeData } from '../../data/guestHomeContext';
  import { writable } from 'svelte/store';
  import GuestCardList from './GuestCardList.svelte';
  import { getLucideIcon } from './guestIconMap';
  import { ArrowRight } from 'lucide-svelte';
  import { resolveGuestItems } from './guestChildren';

  interface Props {
    props: GuestCategoriesProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, fetchCtx, geoCountry, id = 'guest-categories' }: Props = $props();

  const guestHomeStore = $derived(getGuestHomeData() ?? writable<GuestHomeData | undefined>(undefined));
  const guestHome = $derived($guestHomeStore);

  function cloneForContext(template: Block, cat: GuestHomeCategory, index: number): Block {
    const itemProps: GuestCategoryItemProps = {
      ...(template.props as GuestCategoryItemProps),
      title: cat.title,
      description: cat.description,
      href: cat.href,
      icon: cat.icon,
      color: cat.color,
    };
    return { ...template, id: `${template.id}-${cat.id ?? index}`, props: itemProps } as Block;
  }

  const res = $derived(
    resolveGuestItems<GuestHomeCategory>({
      dataSource: props.data_source,
      children: props.children,
      items: props.items,
      contextItems: guestHome?.categories ?? [],
      maxItems: props.max_items ?? 8,
      cloneForContext,
    })
  );

  const childBlocks = $derived(res.manualChildren ?? res.contextChildren);
  const title = $derived(props.title ?? 'Explore What Matters to You');
  const subtitle = $derived(props.subtitle ?? 'Everything you need to achieve your global dreams.');

  function categoryColorClass(color: string): string {
    switch (color) {
      case 'green':
        return 'text-green-600 bg-green-100';
      case 'blue':
        return 'text-blue-600 bg-blue-100';
      case 'purple':
        return 'text-purple-600 bg-purple-100';
      case 'orange':
        return 'text-orange-600 bg-orange-100';
      case 'teal':
        return 'text-teal-600 bg-teal-100';
      default:
        return 'text-[var(--color-primary)] bg-[var(--color-surface-subtle)]';
    }
  }
</script>

<!-- Ref: japaconnect-engineering/sections/16-page-builder.md -->
<GuestCardList
  {title}
  {subtitle}
  view_all_href={props.view_all_href}
  view_all_label="View all categories"
  layout={props.layout ?? 'grid'}
  columns={props.columns ?? 5}
  responsive_columns={props.responsive_columns ?? { tablet: 3, mobile: 1 }}
  gap={props.gap ?? 'lg'}
  card_style={props.card_style ?? 'default'}
  items={res.legacyItems ?? []}
  blocks={childBlocks}
  {fetchCtx}
  {geoCountry}
  {id}
>
  {#snippet card(cat: GuestHomeCategory)}
    {@const Icon = getLucideIcon(cat.icon)}
    <a href={cat.href} class="category-card group">
      <div class="category-icon {categoryColorClass(cat.color)}">
        {#if Icon}
          <Icon size={28} />
        {/if}
      </div>
      <h3 class="category-title">{cat.title}</h3>
      <p class="category-description">{cat.description}</p>
      <span class="category-link">
        Explore {cat.title}
        <ArrowRight size={16} />
      </span>
    </a>
  {/snippet}
</GuestCardList>

<script lang="ts">
  import type { GuestCategoriesProps, GuestHomeData, GuestHomeCategory } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { getGuestHomeData } from '../../data/guestHomeContext';
  import GuestCardList from './GuestCardList.svelte';
  import { getLucideIcon } from './guestIconMap';
  import { ArrowRight } from 'lucide-svelte';

  interface Props {
    props: GuestCategoriesProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, id = 'guest-categories' }: Props = $props();

  const guestHomeStore = getGuestHomeData();
  let guestHome = $state<GuestHomeData | undefined>(undefined);

  $effect(() => {
    if (!guestHomeStore) return;
    const unsub = guestHomeStore.subscribe((v) => (guestHome = v));
    return unsub;
  });

  const items = $derived<GuestHomeCategory[]>(
    props.data_source === 'manual' && props.items
      ? props.items
      : (guestHome?.categories ?? [])
  );

  const visibleItems = $derived(items.slice(0, props.max_items ?? 8));
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
  items={visibleItems}
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

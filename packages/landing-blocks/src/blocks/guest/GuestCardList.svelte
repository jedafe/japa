<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    title?: string;
    subtitle?: string;
    view_all_href?: string;
    view_all_label?: string;
    layout?: 'grid' | 'slider';
    columns?: number;
    responsive_columns?: { tablet?: number; mobile?: number };
    gap?: 'none' | 'xs' | 'sm' | 'md' | 'lg' | 'xl' | '2xl';
    card_style?: 'default' | 'outlined' | 'filled';
    items: any[];
    card: Snippet<[any]>;
    id?: string;
  }

  let {
    title,
    subtitle,
    view_all_href,
    view_all_label = 'View all',
    layout = 'grid',
    columns = 3,
    responsive_columns = {},
    gap = 'md',
    card_style = 'default',
    items,
    card,
    id = 'guest-card-list',
  }: Props = $props();

  const gapMap: Record<NonNullable<Props['gap']>, string> = {
    none: '0',
    xs: '0.5rem',
    sm: '0.75rem',
    md: '1rem',
    lg: '1.5rem',
    xl: '2rem',
    '2xl': '3rem',
  };

  const desktop = $derived(columns);
  const tablet = $derived(responsive_columns.tablet ?? Math.max(2, desktop - 1));
  const mobile = $derived(responsive_columns.mobile ?? 1);
</script>

<section class="guest-section" data-list={id} aria-label={title ?? 'Section'}>
  <div class="guest-section-inner">
    <div class="section-header">
      {#if title}<h2 class="section-title">{title}</h2>{/if}
      {#if subtitle}<p class="section-subtitle">{subtitle}</p>{/if}
    </div>
    <div
      class="card-list"
      class:card-list-grid={layout === 'grid'}
      class:card-list-slider={layout === 'slider'}
      class:card-style-outlined={card_style === 'outlined'}
      class:card-style-filled={card_style === 'filled'}
      style="--gap: {gapMap[gap]}; --desktop: {desktop}; --tablet: {tablet}; --mobile: {mobile};"
    >
      {#each items as item (item.id ?? item.slug ?? item.name ?? item.title)}
        <div class="card-list-cell">
          {@render card(item)}
        </div>
      {/each}
    </div>
    {#if view_all_href}
      <a href={view_all_href} class="section-link">{view_all_label}</a>
    {/if}
  </div>
</section>

<style>
  .guest-section {
    container-type: inline-size;
  }

  .card-list {
    gap: var(--gap, 1rem);
  }

  .card-list-grid {
    display: grid;
    grid-template-columns: repeat(var(--desktop, 3), minmax(0, 1fr));
  }

  .card-list-slider {
    display: flex;
    flex-wrap: nowrap;
    overflow-x: auto;
    scroll-snap-type: x mandatory;
    scrollbar-width: none;
  }

  .card-list-slider::-webkit-scrollbar {
    display: none;
  }

  .card-list-slider .card-list-cell {
    flex: 0 0 calc((100% - (var(--desktop, 3) - 1) * var(--gap, 1rem)) / var(--desktop, 3));
    scroll-snap-align: start;
  }

  @media (max-width: 1024px) {
    .card-list-grid {
      grid-template-columns: repeat(var(--tablet, 2), minmax(0, 1fr));
    }
    .card-list-slider .card-list-cell {
      flex: 0 0 calc((100% - (var(--tablet, 2) - 1) * var(--gap, 1rem)) / var(--tablet, 2));
    }
  }

  @media (max-width: 768px) {
    .card-list-grid {
      grid-template-columns: repeat(var(--mobile, 1), minmax(0, 1fr));
    }
    .card-list-slider .card-list-cell {
      flex: 0 0 calc((100% - (var(--mobile, 1) - 1) * var(--gap, 1rem)) / var(--mobile, 1));
    }
  }
</style>

<script lang="ts">
  // Ref: japaconnect-engineering/sections/16-page-builder.md
  // Single category card inside `guest_categories`, rendered as a child block
  // so each card is selectable/editable in the page builder tree.
  import type { GuestCategoryItemProps } from '../../types';
  import { ArrowRight } from 'lucide-svelte';
  import { getLucideIcon } from './guestIconMap';

  interface Props {
    props: GuestCategoryItemProps;
    id?: string;
  }

  let { props, id }: Props = $props();

  const Icon = $derived(getLucideIcon(props.icon));
  const title = $derived(props.title ?? '');
  const href = $derived(props.href ?? '#');

  function categoryColorClass(color: string | undefined): string {
    switch (color) {
      case 'green': return 'text-green-600 bg-green-100';
      case 'blue': return 'text-blue-600 bg-blue-100';
      case 'purple': return 'text-purple-600 bg-purple-100';
      case 'orange': return 'text-orange-600 bg-orange-100';
      case 'teal': return 'text-teal-600 bg-teal-100';
      default: return 'text-[var(--color-primary)] bg-[var(--color-surface-subtle)]';
    }
  }
</script>

<a href={href} class="category-card group" {id}>
  <div class="category-icon {categoryColorClass(props.color)}">
    {#if Icon}
      <Icon size={28} />
    {/if}
  </div>
  <h3 class="category-title">{title}</h3>
  <p class="category-description">{props.description ?? ''}</p>
  <span class="category-link">
    Explore {title}
    <ArrowRight class="rtl:-scale-x-100" size={16} />
  </span>
</a>

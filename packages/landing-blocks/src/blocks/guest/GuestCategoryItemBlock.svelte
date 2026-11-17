<script lang="ts">
  // Ref: japaconnect-engineering/sections/16-page-builder.md
  // Single category card inside `guest_categories`, rendered as a child block
  // so each card is selectable/editable in the page builder tree.
  import type { GuestCategoryItemProps } from '../../types';
  import { ArrowRight } from 'lucide-svelte';
  import { getLucideIcon } from './guestIconMap';
  import { chipClass } from './chips';

  interface Props {
    props: GuestCategoryItemProps;
    id?: string;
  }

  let { props, id }: Props = $props();

  const Icon = $derived(getLucideIcon(props.icon));
  const title = $derived(props.title ?? '');
  const href = $derived(props.href ?? '#');

  function categoryColorClass(color: string | undefined): string {
    return chipClass(color);
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

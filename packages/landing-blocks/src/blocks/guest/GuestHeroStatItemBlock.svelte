<script lang="ts">
  // Ref: japaconnect-engineering/sections/16-page-builder.md
  // Single stat inside `guest_hero_stats` — icon + value + label, rendered as
  // a child block so each stat is editable in the page builder tree.
  import type { GuestHeroStatItemProps } from '../../types';
  import { getLucideIcon } from './guestIconMap';

  interface Props {
    props: GuestHeroStatItemProps;
    id?: string;
  }

  let { props, id }: Props = $props();

  const Icon = $derived(getLucideIcon(props.icon ?? 'Users'));
</script>

<div class="hero-stat" {id}>
  <div class="hero-stat-icon">
    {#if Icon}
      <Icon size={20} />
    {/if}
  </div>
  <div class="hero-stat-body">
    <span class="hero-stat-value">{props.value ?? ''}</span>
    <span class="hero-stat-label">{props.label ?? ''}</span>
  </div>
</div>

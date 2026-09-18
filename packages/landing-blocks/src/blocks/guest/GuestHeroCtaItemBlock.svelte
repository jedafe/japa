<script lang="ts">
  // Ref: japaconnect-engineering/sections/16-page-builder.md
  // Single CTA button inside `guest_hero_text`. Rendered as a child block so
  // each hero action is selectable/editable in the page builder tree.
  import type { GuestHeroCtaItemProps } from '../../types';
  import { getLucideIcon } from './guestIconMap';

  interface Props {
    props: GuestHeroCtaItemProps;
    id?: string;
  }

  let { props, id }: Props = $props();

  const href = $derived(props.href ?? '#');
  const label = $derived(props.label ?? '');
  const Icon = $derived(getLucideIcon(props.icon));
</script>

<a href={href} class="hero-btn" class:hero-btn-primary={props.variant === 'primary'} class:hero-btn-secondary={props.variant !== 'primary'} {id}>
  {label}
  {#if Icon}
    <Icon size={18} />
  {:else if props.icon}
    <span class="hero-btn-icon-label">{props.icon}</span>
  {/if}
</a>

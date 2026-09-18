<script lang="ts">
  // Ref: japaconnect-engineering/sections/16-page-builder.md
  // Single tool card inside `guest_tools`, rendered as a child block so each
  // tool is selectable/editable in the page builder tree.
  import type { GuestToolItemProps } from '../../types';
  import { getLucideIcon } from './guestIconMap';

  interface Props {
    props: GuestToolItemProps;
    id?: string;
  }

  let { props, id }: Props = $props();

  const Icon = $derived(getLucideIcon(props.icon));
  const href = $derived(props.href ?? (props.slug ? `/tools/${props.slug}` : '/'));
</script>

<a href={href} class="tool-card group" {id}>
  <div
    class="tool-icon"
    style="background-color: {props.accent ? `color-mix(in srgb, ${props.accent} 15%, transparent)` : 'var(--color-surface-subtle)'}; color: {props.accent ?? 'var(--color-primary)'}"
  >
    {#if Icon}
      <Icon size={24} />
    {/if}
  </div>
  <h3 class="tool-name">{props.name ?? ''}</h3>
  <p class="tool-description">{props.description ?? ''}</p>
</a>

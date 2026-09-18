<script lang="ts">
  // Ref: japaconnect-engineering/sections/16-page-builder.md
  // Single link inside a `guest_footer_column` or the footer bottom bar.
  // The wrapping <li> is provided by the parent list.
  import type { GuestFooterLinkProps } from '../../types';
  import { getLucideIcon } from './guestIconMap';

  interface Props {
    props: GuestFooterLinkProps;
    id?: string;
  }

  let { props, id }: Props = $props();

  const LinkIcon = $derived(getLucideIcon(props.icon));

  function isExternal(url: string | undefined): boolean {
    return !!url && (/^https?:\/\//.test(url) || /^\/\//.test(url));
  }

  const attrs = $derived.by(() => {
    const shouldBlank = props.target === '_blank' || isExternal(props.url);
    return {
      href: props.url ?? '',
      target: shouldBlank ? '_blank' : undefined,
      rel: shouldBlank ? 'noopener noreferrer' : undefined,
    };
  });
</script>

{#if props.url}
  <a {...attrs} {id}>
    {#if LinkIcon}
      <LinkIcon size={14} />
    {/if}
    <span>{props.label ?? ''}</span>
  </a>
{:else}
  <span class="footer-column-text" {id}>
    {#if LinkIcon}
      <LinkIcon size={14} />
    {/if}
    <span>{props.label ?? ''}</span>
  </span>
{/if}

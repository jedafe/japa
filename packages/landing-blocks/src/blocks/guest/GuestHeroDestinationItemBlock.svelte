<script lang="ts">
  import type { GuestHeroDestinationItemProps } from '../../types';
  import { Globe } from 'lucide-svelte';
  import { countryCodeToEmoji } from '../../lib/countryEmoji';

  interface Props {
    props: GuestHeroDestinationItemProps;
    id?: string;
  }

  let { props, id }: Props = $props();

  const name = $derived(props.name ?? '');
  const href = $derived(props.href ?? '#' );
  const iconUrl = $derived(props.icon_url);
  const code = $derived(props.code);

  const emoji = $derived(countryCodeToEmoji(code));
</script>

<!-- Ref: japaconnect-engineering/sections/16-page-builder.md -->
<a href={href} class="destination-row" {id}>
  <span class="destination-flag">
    {#if iconUrl}
      <img src={iconUrl} alt="" class="destination-flag-img" />
    {:else if emoji}
      <span class="destination-flag-emoji" aria-hidden="true">{emoji}</span>
    {:else}
      <Globe size={16} />
    {/if}
  </span>
  <span class="destination-name">{name}</span>
</a>

<style>
  .destination-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    color: var(--color-text);
    text-decoration: none;
  }
  .destination-row:hover {
    color: var(--color-primary);
  }
  .destination-flag {
    flex: 0 0 1.25rem;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .destination-flag-img {
    width: 1.25rem;
    height: 1.25rem;
    border-radius: 9999px;
    object-fit: cover;
  }
  .destination-name {
    font-size: 0.875rem;
    font-weight: 500;
  }
</style>

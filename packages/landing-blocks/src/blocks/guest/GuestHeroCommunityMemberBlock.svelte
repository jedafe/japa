<script lang="ts">
  import type { GuestHeroCommunityMemberProps } from '../../types';
  import { Users } from 'lucide-svelte';

  interface Props {
    props: GuestHeroCommunityMemberProps;
    id?: string;
  }

  let { props, id }: Props = $props();

  const href = $derived(props.href ?? undefined);
  const imageUrl = $derived(props.image_url);
</script>

{#if href}
  <a {id} href={href} class="community-avatar" aria-label={props.name || 'Community member'}>
    {#if imageUrl}
      <img src={imageUrl} alt={props.name ?? ''} class="community-avatar-img" />
    {:else}
      <Users size={16} />
    {/if}
  </a>
{:else}
  <div {id} class="community-avatar" aria-label={props.name || 'Community member'}>
    {#if imageUrl}
      <img src={imageUrl} alt={props.name ?? ''} class="community-avatar-img" />
    {:else}
      <Users size={16} />
    {/if}
  </div>
{/if}

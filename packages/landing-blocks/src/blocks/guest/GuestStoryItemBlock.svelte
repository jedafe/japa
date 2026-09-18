<script lang="ts">
  // Ref: japaconnect-engineering/sections/16-page-builder.md
  // Single story card inside `guest_stories`, rendered as a child block so
  // each story is selectable/editable in the page builder tree.
  import type { GuestStoryItemProps } from '../../types';

  interface Props {
    props: GuestStoryItemProps;
    id?: string;
  }

  let { props, id }: Props = $props();

  function initials(name: string | undefined): string {
    return (name ?? '')
      .split(' ')
      .filter(Boolean)
      .map((w) => w[0])
      .slice(0, 2)
      .join('')
      .toUpperCase();
  }

  const backgroundStyle = $derived(
    props.author_avatar_url
      ? `background-image: linear-gradient(rgba(15,23,42,0.55), rgba(15,23,42,0.85)), url(${props.author_avatar_url}); background-size: cover; background-position: center;`
      : undefined
  );
</script>

<div class="story-card" style={backgroundStyle} {id}>
  <div class="story-quote">“{props.quote ?? ''}”</div>
  <div class="story-author">
    <div class="story-avatar">
      {#if props.author_avatar_url}
        <img src={props.author_avatar_url} alt="" class="story-avatar-img" />
      {:else}
        <div class="initials-avatar" aria-hidden="true">{initials(props.author_name)}</div>
      {/if}
    </div>
    <div class="story-author-body">
      <span class="story-author-name">{props.author_name ?? ''}</span>
      {#if props.author_role}
        <span class="story-author-role">{props.author_role}</span>
      {/if}
    </div>
  </div>
</div>

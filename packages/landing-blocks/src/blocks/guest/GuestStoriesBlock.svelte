<script lang="ts">
  import type { GuestStoriesProps, GuestHomeData, GuestHomeStory } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { getGuestHomeData } from '../../data/guestHomeContext';
  import GuestCardList from './GuestCardList.svelte';
  import { ArrowRight } from 'lucide-svelte';

  interface Props {
    props: GuestStoriesProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, id = 'guest-stories' }: Props = $props();

  const guestHomeStore = getGuestHomeData();
  let guestHome: GuestHomeData | undefined = $state(undefined);

  $effect(() => {
    if (!guestHomeStore) return;
    const unsub = guestHomeStore.subscribe((v) => (guestHome = v));
    return unsub;
  });

  const items = $derived<GuestHomeStory[]>(
    props.data_source === 'manual' && props.items
      ? props.items
      : (guestHome?.stories ?? [])
  );

  const visibleItems = $derived(items.slice(0, props.max_items ?? 6));
  const title = $derived(props.title ?? 'Success Stories That Inspire');
  const subtitle = $derived(props.subtitle ?? 'From landing dream jobs to getting scholarships and starting new lives—our community is achieving greatness worldwide.');

  function initials(name: string): string {
    return name
      .split(' ')
      .filter(Boolean)
      .map((w) => w[0])
      .slice(0, 2)
      .join('')
      .toUpperCase();
  }

  function backgroundStyle(story: GuestHomeStory): string | undefined {
    if (!story.author_avatar_url) return undefined;
    return `background-image: linear-gradient(rgba(15,23,42,0.55), rgba(15,23,42,0.85)), url(${story.author_avatar_url}); background-size: cover; background-position: center;`;
  }
</script>

<!-- Ref: japaconnect-engineering/sections/16-page-builder.md -->
<GuestCardList
  {title}
  {subtitle}
  view_all_href={props.view_all_href ?? '/stories'}
  view_all_label="Read more stories"
  layout={props.layout ?? 'slider'}
  columns={props.columns ?? 3}
  responsive_columns={props.responsive_columns}
  gap={props.gap ?? 'md'}
  card_style={props.card_style ?? 'default'}
  items={visibleItems}
  {id}
>
  {#snippet card(story: GuestHomeStory)}
    <div class="story-card" role="listitem" style={backgroundStyle(story)}>
      <div class="story-quote">“{story.quote}”</div>
      <div class="story-author">
        <div class="story-avatar">
          {#if story.author_avatar_url}
            <img src={story.author_avatar_url} alt="" class="story-avatar-img" />
          {:else}
            {initials(story.author_name)}
          {/if}
        </div>
        <div class="story-author-body">
          <span class="story-author-name">{story.author_name}</span>
          {#if story.author_role}
            <span class="story-author-role">{story.author_role}</span>
          {/if}
        </div>
      </div>
    </div>
  {/snippet}
</GuestCardList>

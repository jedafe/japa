<script lang="ts">
  import type { GuestStoriesProps, GuestHomeData, GuestHomeStory } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { getGuestHomeData } from '../../data/guestHomeContext';
  import { writable } from 'svelte/store';
  import { ChevronLeft, ChevronRight, ArrowRight } from 'lucide-svelte';

  interface Props {
    props: GuestStoriesProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, id = 'guest-stories' }: Props = $props();

  const guestHomeStore = $derived(getGuestHomeData() ?? writable<GuestHomeData | undefined>(undefined));
  const guestHome = $derived($guestHomeStore);
  let scroller = $state<HTMLDivElement | undefined>(undefined);
  let activeIndex = $state(0);

  const items = $derived<GuestHomeStory[]>(
    props.data_source === 'manual' && props.items
      ? props.items
      : (guestHome?.stories ?? [])
  );

  const visibleItems = $derived(items.slice(0, props.max_items ?? 3));
  const eyebrow = $derived(props.eyebrow ?? 'Real People. Real Stories.');
  const title = $derived(props.title ?? 'Success Stories That Inspire');
  const subtitle = $derived(
    props.subtitle ??
      'From landing dream jobs to getting scholarships and starting new lives—our community is achieving greatness worldwide.'
  );
  const ctaLabel = $derived(props.cta_label ?? 'Read More Stories');
  const ctaHref = $derived(props.cta_href ?? '/stories');

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

  function scrollBy(direction: number) {
    if (!scroller) return;
    const card = scroller.firstElementChild as HTMLElement | null;
    const width = card?.offsetWidth ?? 420;
    scroller.scrollBy({ left: direction * (width + 16), behavior: 'smooth' });
  }

  function goTo(i: number) {
    activeIndex = i;
    if (!scroller) return;
    const card = scroller.children[i] as HTMLElement | undefined;
    if (card) card.scrollIntoView({ behavior: 'smooth', inline: 'start', block: 'nearest' });
  }
</script>

<!-- Ref: japaconnect-engineering/sections/16-page-builder.md -->
<section class="guest-section stories-section" {id} aria-label={title}>
  <div class="guest-section-inner stories-inner">
    <div class="stories-header">
      <span class="stories-eyebrow">{eyebrow}</span>
      <h2 class="section-title section-title-light">{title}</h2>
      <p class="stories-subtitle">{subtitle}</p>
      <a href={ctaHref} class="stories-cta">
        {ctaLabel}
        <ArrowRight size={18} />
      </a>
    </div>

    <div class="carousel-wrap stories-carousel-wrap">
      <button
        type="button"
        class="carousel-arrow carousel-arrow-left stories-arrow"
        onclick={() => scrollBy(-1)}
        aria-label="Previous"
      >
        <ChevronLeft size={20} />
      </button>

      <div bind:this={scroller} class="story-scroll">
        {#each visibleItems as story (story.id)}
          <div class="story-card" style={backgroundStyle(story)}>
            <div class="story-quote">“{story.quote}”</div>
            <div class="story-author">
              <div class="story-avatar">
                {#if story.author_avatar_url}
                  <img src={story.author_avatar_url} alt="" class="story-avatar-img" />
                {:else}
                  <div class="initials-avatar" aria-hidden="true">{initials(story.author_name)}</div>
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
        {/each}
      </div>

      <button
        type="button"
        class="carousel-arrow carousel-arrow-right stories-arrow"
        onclick={() => scrollBy(1)}
        aria-label="Next"
      >
        <ChevronRight size={20} />
      </button>
    </div>

    <div class="stories-dots">
      {#each visibleItems as _, i (i)}
        <button
          type="button"
          class="story-dot {i === activeIndex ? 'story-dot-active' : ''}"
          onclick={() => goTo(i)}
          aria-label="Go to story {i + 1}"
        ></button>
      {/each}
    </div>
  </div>
</section>

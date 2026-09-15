<script lang="ts">
  import type { GuestHeroImageProps, MediaRef } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { ChevronLeft, ChevronRight } from 'lucide-svelte';
  import { onMount } from 'svelte';

  interface Props {
    props: GuestHeroImageProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, id = 'guest-hero-image' }: Props = $props();

  const defaultSlides: MediaRef[] = [
    { media_id: 'diaspora-community', url: '/brand/diaspora-community.jpg', alt: 'A confident traveler at the airport' },
    { media_id: 'kigali-airplane-window', url: '/brand/kigali-airplane-window.jpg', alt: 'Airplane window view over a new city' },
  ];

  const slides = $derived(props.slides && props.slides.length > 0 ? props.slides : defaultSlides);
  const autoplay = $derived(props.autoplay ?? true);
  const interval = $derived(props.interval ?? 5000);
  const dots = $derived(props.dots ?? true);
  const arrows = $derived(props.arrows ?? true);

  let index = $state(0);
  let timer: ReturnType<typeof setInterval> | undefined = $state(undefined);

  function go(n: number) {
    index = (n + slides.length) % slides.length;
  }

  function next() {
    go(index + 1);
  }

  function prev() {
    go(index - 1);
  }

  onMount(() => {
    if (autoplay && slides.length > 1) {
      timer = setInterval(next, interval);
    }
    return () => {
      if (timer) clearInterval(timer);
    };
  });
</script>

<!-- Ref: japaconnect-engineering/sections/16-page-builder.md -->
<div class="hero-image-wrap" {id}>
  {#each slides as slide, i (slide.media_id)}
    {#if i === index}
      <img
        src={slide.url}
        alt={slide.alt}
        class="hero-image"
        loading={i === 0 ? 'eager' : 'lazy'}
        decoding="async"
      />
    {/if}
  {/each}

  {#if arrows && slides.length > 1}
    <button
      type="button"
      class="carousel-arrow carousel-arrow-left"
      onclick={prev}
      aria-label="Previous slide"
    >
      <ChevronLeft size={20} />
    </button>
    <button
      type="button"
      class="carousel-arrow carousel-arrow-right"
      onclick={next}
      aria-label="Next slide"
    >
      <ChevronRight size={20} />
    </button>
  {/if}

  {#if dots && slides.length > 1}
    <div class="hero-dots">
      {#each slides as _, i (i)}
        <button
          type="button"
          class="hero-dot"
          class:hero-dot-active={i === index}
          onclick={() => go(i)}
          aria-label="Go to slide {i + 1}"
        ></button>
      {/each}
    </div>
  {/if}
</div>

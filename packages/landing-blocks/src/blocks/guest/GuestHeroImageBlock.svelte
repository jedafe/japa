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
  ];

  const slides = $derived(props.slides && props.slides.length > 0 ? props.slides : defaultSlides);
  const autoplay = $derived(props.autoplay ?? false);
  const interval = $derived(props.interval ?? 5000);
  const dots = $derived(props.dots ?? false);
  const arrows = $derived(props.arrows ?? false);

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

<style>
  .hero-dots {
    position: absolute;
    bottom: 1rem;
    left: 50%;
    transform: translateX(-50%);
    display: flex;
    gap: 0.5rem;
    z-index: 10;
  }
  .hero-dot {
    width: 0.625rem;
    height: 0.625rem;
    border-radius: 9999px;
    background: rgba(255, 255, 255, 0.5);
    border: 0;
    cursor: pointer;
    transition: background 0.15s;
  }
  .hero-dot:hover,
  .hero-dot-active {
    background: rgba(255, 255, 255, 1);
  }
</style>

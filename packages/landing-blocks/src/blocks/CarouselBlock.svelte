<script lang="ts">
  import type { CarouselProps, Block } from '../types';
  import { blockRegistry } from '../render/blockRegistry';
  import BlockShell from '../shell/BlockShell.svelte';

  interface Props {
    props: CarouselProps;
  }
  let { props }: Props = $props();

  let current = $state(0);
  let paused = $state(false);

  const slideCount = $derived(props.slides.length);

  function next() {
    current = (current + 1) % slideCount;
  }
  function prev() {
    current = (current - 1 + slideCount) % slideCount;
  }
  function goTo(idx: number) {
    current = idx;
  }

  // Autoplay
  $effect(() => {
    if (!props.autoplay || paused || slideCount <= 1) return;
    const interval = setInterval(next, props.interval ?? 5000);
    return () => clearInterval(interval);
  });
</script>

<div
  class="relative overflow-hidden px-6 py-16"
  role="region"
  aria-label="Carousel"
  onmouseenter={() => (paused = props.pause_on_hover)}
  onmouseleave={() => (paused = false)}
>
  <div class="mx-auto max-w-6xl">
    <div class="relative overflow-hidden rounded-[var(--radius-card)]">
      {#each props.slides as slide, i}
        {@const meta = blockRegistry[slide.type]}
        <div
          class="transition-opacity duration-500 {i === current ? 'opacity-100' : 'absolute inset-0 opacity-0'}"
        >
          {#if meta}
            {@const Component = meta.component}
            <Component props={slide.props} />
          {/if}
        </div>
      {/each}
    </div>

    {#if props.arrows && slideCount > 1}
      <button
        onclick={prev}
        aria-label="Previous slide"
        class="absolute left-2 top-1/2 z-20 -translate-y-1/2 rounded-full bg-[var(--color-surface)] p-2 shadow-md hover:bg-[var(--color-surface-subtle)]"
      >
        &#8592;
      </button>
      <button
        onclick={next}
        aria-label="Next slide"
        class="absolute right-2 top-1/2 z-20 -translate-y-1/2 rounded-full bg-[var(--color-surface)] p-2 shadow-md hover:bg-[var(--color-surface-subtle)]"
      >
        &#8594;
      </button>
    {/if}

    {#if props.dots && slideCount > 1}
      <div class="mt-4 flex justify-center gap-2">
        {#each props.slides as _, i}
          <button
            onclick={() => goTo(i)}
            aria-label="Go to slide {i + 1}"
            class="h-2 w-2 rounded-full transition-all {i === current ? 'bg-[var(--color-primary)] w-6' : 'bg-[var(--color-border)]'}"
          ></button>
        {/each}
      </div>
    {/if}
  </div>
</div>

<script lang="ts">
  import type { CarouselProps } from '../types';
  import type { FetchContext } from '../data/types';
  import { blockComponentFor } from '../render/blockComponent';
  import { isBlockVisible, isBlockEmpty } from '../render/blockVisibility';
  import BlockShell from '../shell/BlockShell.svelte';
  import ScopedCss from '../shell/ScopedCss.svelte';
  import { responsiveVisibilityCss } from '../shell/styleToCss';

  interface Props {
    props: CarouselProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }
  let { props, fetchCtx, geoCountry }: Props = $props();

  let current = $state(0);
  let paused = $state(false);

  const slideCount = $derived(props.slides.length);
  const visibilityCss = $derived(responsiveVisibilityCss(props.slides));

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

<ScopedCss css={visibilityCss} />

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
        {@const meta = blockComponentFor(slide.type)}
        {#if isBlockVisible(slide, geoCountry) && meta}
          {@const Component = meta.component}
          <div
            class="transition-opacity duration-500 {i === current ? 'opacity-100' : 'absolute inset-0 opacity-0 pointer-events-none'}"
          >
            <BlockShell
              style={slide.style}
              motion={slide.motion}
              background={slide.style?.background}
              hideIfEmpty={slide.visibility_rules?.hide_if_empty}
              initialHasContent={!isBlockEmpty(slide)}
              scopeKey={slide.id}
            >
              {#if slide.type === 'custom'}
                <Component props={slide.props} blockDefId={slide.block_def_id} {fetchCtx} {geoCountry} id={slide.id} />
              {:else}
                <Component props={slide.props} {fetchCtx} {geoCountry} id={slide.id} />
              {/if}
            </BlockShell>
          </div>
        {/if}
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

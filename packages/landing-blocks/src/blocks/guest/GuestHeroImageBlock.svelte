<script lang="ts">
  import type { GuestHeroImageProps, MediaRef, Block, BlockStyle } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { ChevronLeft, ChevronRight } from 'lucide-svelte';
  import { blockRegistry } from '../../render/blockRegistry';
  import BlockShell from '../../shell/BlockShell.svelte';
  import { isBlockVisible, isBlockEmpty } from '../../render/blockVisibility';

  interface Props {
    props: GuestHeroImageProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, fetchCtx, geoCountry, id = 'guest-hero-image' }: Props = $props();

  const defaultSlides: MediaRef[] = [
    { media_id: 'diaspora-community', url: '/brand/diaspora-community.jpg', alt: 'A confident traveler at the airport' },
  ];

  const defaultChildren: Block[] = $derived([
    { id: `${id}-destinations`, type: 'guest_hero_destinations', visible: true, props: {} },
    { id: `${id}-community`, type: 'guest_hero_community', visible: true, props: {} },
  ]);

  const children = $derived(props.children ?? defaultChildren);

  const slides = $derived(props.slides && props.slides.length > 0 ? props.slides : defaultSlides);
  const autoplay = $derived(props.autoplay ?? true);
  const interval = $derived(props.interval ?? 5000);
  const dots = $derived(props.dots ?? true);
  const arrows = $derived(props.arrows ?? true);

  let index = $state(0);

  function go(n: number) {
    if (slides.length === 0) return;
    index = ((n % slides.length) + slides.length) % slides.length;
  }

  function next() {
    go(index + 1);
  }

  function prev() {
    go(index - 1);
  }

  $effect(() => {
    if (!autoplay || slides.length <= 1) return;
    const t = setInterval(next, interval);
    return () => clearInterval(t);
  });

  // Deep-merge helper so user-supplied block styles (e.g. top/left from the style tab)
  // only override the matching default fields, not the whole default style group.
  function deepMerge<T>(a: T | undefined, b: T | undefined): T | undefined {
    if (a === undefined) return b;
    if (b === undefined) return a;
    if (Array.isArray(a) || Array.isArray(b)) return b as T;
    if (typeof a === 'object' && typeof b === 'object') {
      const result = { ...a } as Record<string, unknown>;
      for (const key of Object.keys(b as Record<string, unknown>)) {
        result[key] = deepMerge(result[key], (b as Record<string, unknown>)[key]);
      }
      return result as T;
    }
    return b as T;
  }

  function defaultStyle(type: string): BlockStyle | undefined {
    if (type === 'guest_hero_destinations') {
      // The BlockShell wrapper is the absolutely-positioned floating card.
      // The inner .floating-card-destinations div is width: 100% and has no offsets,
      // so the style-tab top/left are not added to a legacy CSS offset.
      return {
        layout: { position: 'absolute', top: 24, left: 24, z_index: 10, width: 220 },
        responsive: {
          tablet: { layout: { top: 16, left: 16, width: 200 } },
          mobile: { layout: { top: 12, left: 12, width: 170 } }
        }
      };
    }
    if (type === 'guest_hero_community') {
      return {
        layout: { position: 'absolute', bottom: 40, right: 24, z_index: 10, width: 260 },
        responsive: {
          tablet: { layout: { bottom: 24, right: 16, width: 220 } },
          mobile: { layout: { bottom: 16, right: 12, width: 200 } }
        }
      };
    }
    return undefined;
  }

  function childStyle(block: Block): BlockStyle | undefined {
    return deepMerge(defaultStyle(block.type), block.style);
  }

  function renderableComponent(block: Block) {
    const meta = blockRegistry[block.type];
    if (!meta || !isBlockVisible(block, geoCountry)) return null;
    return { Component: meta.component, style: childStyle(block) };
  }
</script>

<!-- Ref: japaconnect-engineering/sections/16-page-builder.md -->
<div class="hero-image-wrap" {id}>
  {#each slides as slide, i (i)}
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

  {#each children as child (child.id)}
    {@const r = renderableComponent(child)}
    {#if r}
      {@const Component = r.Component}
      <BlockShell style={r.style} motion={child.motion} background={child.style?.background} hideIfEmpty={child.visibility_rules?.hide_if_empty} initialHasContent={!isBlockEmpty(child)}>
        <Component props={child.props} {fetchCtx} {geoCountry} id={child.id} />
      </BlockShell>
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

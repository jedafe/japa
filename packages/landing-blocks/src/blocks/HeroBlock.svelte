<script lang="ts">
  import { onMount } from 'svelte';
  import type { HeroProps } from '../types';
  import GuestHeroIllustration from './GuestHeroIllustration.svelte';
  import { ChevronLeft, ChevronRight } from 'lucide-svelte';

  interface Props {
    props: HeroProps;
  }
  let { props }: Props = $props();

  const layout = $derived(props.layout ?? 'center');

  function headlineParts(headline: string, highlight?: string) {
    if (!highlight) return null;
    const idx = headline.indexOf(highlight);
    if (idx === -1) return null;
    return {
      before: headline.slice(0, idx),
      match: highlight,
      after: headline.slice(idx + highlight.length)
    };
  }

  const splitHeadline = $derived(headlineParts(props.headline, props.highlight));

  const hasSlider = $derived((props.slides?.length ?? 0) > 1);
  let activeIndex = $state(0);

  function goTo(index: number) {
    if (!props.slides || props.slides.length === 0) return;
    activeIndex = (index + props.slides.length) % props.slides.length;
  }
  function next() {
    if (!props.slides || props.slides.length === 0) return;
    activeIndex = (activeIndex + 1) % props.slides.length;
  }
  function prev() {
    if (!props.slides || props.slides.length === 0) return;
    activeIndex = (activeIndex - 1 + props.slides.length) % props.slides.length;
  }

  onMount(() => {
    const mql = window.matchMedia('(prefers-reduced-motion: reduce)');
    if (mql.matches || !props.slides || props.slides.length <= 1) return;
    const id = setInterval(() => {
      if (props.slides) {
        activeIndex = (activeIndex + 1) % props.slides.length;
      }
    }, 5000);
    return () => clearInterval(id);
  });


</script>

{#if layout === 'split'}
  <section class="relative px-6 py-12 lg:py-20">
    <div class="mx-auto max-w-7xl">
      <!-- Media: stacked above the text on mobile; anchored to the right
           half of the card on lg+ so it never sits behind the copy. -->
      <div class="hero-split-card relative w-full overflow-hidden rounded-[var(--radius-card)] shadow-xl lg:h-[560px]">
        <div class="relative h-52 w-full pt-4 sm:h-64 lg:absolute lg:inset-y-0 lg:end-0 lg:h-full lg:w-[62%] lg:pe-8 lg:pt-0">
          {#if hasSlider}
            <div
              class="flex h-full w-full transition-transform duration-500 motion-reduce:transition-none"
              style="transform: translateX(-{activeIndex * 100}%)"
            >
              {#each props.slides ?? [] as slide, i (slide.url + i)}
                <div class="relative h-full w-full shrink-0">
                  <img
                    src={slide.url}
                    alt={slide.alt}
                    class="h-full w-full object-contain lg:object-cover"
                    loading={i === 0 ? 'eager' : 'lazy'}
                  />
                </div>
              {/each}
            </div>
          {:else if props.media}
            <img
              src={props.media.url}
              alt={props.media.alt}
              class="h-full w-full object-contain lg:object-cover"
              loading="eager"
            />
          {:else}
            <GuestHeroIllustration />
          {/if}
          <!-- Mobile: soft blend into the card below. Desktop: fade the
               media's left edge into the card background — the image sits
               beside the text, not behind it. -->
          <div class="hero-media-fade absolute inset-0"></div>

          {#if hasSlider}
            <!-- Prev / Next arrows — anchored to the media region in both modes -->
            <div class="absolute inset-y-0 start-0 z-20 flex items-center ps-2 sm:ps-4">
              <button
                type="button"
                onclick={prev}
                aria-label="Previous slide"
                class="hero-arrow flex h-10 w-10 items-center justify-center rounded-full shadow-lg backdrop-blur-sm transition"
              >
                {#if ChevronLeft}
                  <ChevronLeft class="rtl:-scale-x-100" size={20} />
                {:else}
                  <span aria-hidden="true">&#10094;</span>
                {/if}
              </button>
            </div>
            <div class="absolute inset-y-0 end-0 z-20 flex items-center pe-2 sm:pe-4">
              <button
                type="button"
                onclick={next}
                aria-label="Next slide"
                class="hero-arrow flex h-10 w-10 items-center justify-center rounded-full shadow-lg backdrop-blur-sm transition"
              >
                {#if ChevronRight}
                  <ChevronRight class="rtl:-scale-x-100" size={20} />
                {:else}
                  <span aria-hidden="true">&#10095;</span>
                {/if}
              </button>
            </div>

            <!-- Dots -->
            <div class="absolute bottom-5 end-4 z-20 flex gap-2 sm:end-6 lg:end-8">
              {#each props.slides ?? [] as _, i}
                <button
                  type="button"
                  onclick={() => goTo(i)}
                  aria-label="Go to slide {i + 1}"
                  aria-current={i === activeIndex}
                  class="h-2.5 w-2.5 rounded-full transition {i === activeIndex ? 'bg-[var(--color-primary)] scale-110' : 'bg-[var(--color-surface)]/80 hover:bg-[var(--color-surface)]'}"
                ></button>
              {/each}
            </div>
          {/if}
        </div>

        <!-- Text content — capped to the left half on lg+ so it can't
             reach under the media. -->
        <div class="relative z-10 flex flex-col justify-center px-8 pb-8 pt-5 sm:px-10 sm:pt-6 lg:h-full lg:max-w-[50%] lg:px-16 lg:py-0">
          {#if props.eyebrow}
            <p class="hero-eyebrow mb-4 text-sm font-semibold uppercase tracking-wide">
              {props.eyebrow}
            </p>
          {/if}

          <h1
            class="hero-heading text-balance text-4xl font-bold tracking-tight sm:text-5xl lg:text-6xl"
            style="font-family: var(--block-heading-family, var(--font-heading, var(--font-sans))); font-size: var(--block-heading-size); font-weight: var(--block-heading-weight);"
          >
            {#if splitHeadline}
              {splitHeadline.before}<span class="text-[var(--color-success)]">{splitHeadline.match}</span>{splitHeadline.after}
            {:else}
              {props.headline}
            {/if}
          </h1>

          {#if props.tagline}
            <p class="hero-heading mt-4 text-balance text-lg font-medium sm:text-xl">
              {props.tagline}
            </p>
          {/if}

          {#if props.subhead}
            <p
              class="mt-4 text-balance text-lg text-[var(--color-text-subtle)]"
              style="font-family: var(--block-body-family); font-size: var(--block-body-size); font-weight: var(--block-body-weight);"
            >
              {props.subhead}
            </p>
          {/if}

          {#if props.ctas && props.ctas.length > 0}
            <div class="mt-8 flex flex-wrap items-center gap-4">
              {#each props.ctas as cta, i (cta.href + cta.label)}
                <a
                  href={cta.href}
                  class="inline-flex items-center justify-center rounded-full px-7 py-3.5 text-sm font-semibold transition-all duration-200
                    {cta.variant === 'outline'
                      ? 'border-2 border-[var(--color-primary)] text-[var(--color-primary)] hover:bg-[var(--color-primary)] hover:text-[var(--color-primary-contrast)]'
                      : cta.variant === 'ghost'
                        ? 'text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'
                        : 'bg-gradient-to-r from-[#2563eb] to-[#16a34a] text-white shadow-md hover:shadow-lg hover:from-[#1d4ed8] hover:to-[#15803d]'
                    }"
                >
                  {cta.label}
                </a>
              {/each}
            </div>
          {/if}

          {#if props.trust_text}
            <p class="mt-5 text-sm text-[var(--color-muted)]">{props.trust_text}</p>
          {/if}
        </div>
      </div>
    </div>
  </section>
{:else}
  <section class="relative flex min-h-[480px] flex-col items-center justify-center px-6 py-20 text-center">
    <div class="relative z-10 mx-auto max-w-4xl">
      {#if props.eyebrow}
        <p class="mb-4 text-sm font-semibold uppercase tracking-wide text-[var(--color-primary)]">
          {props.eyebrow}
        </p>
      {/if}
      {#if props.headline}
        <h1
          class="text-balance text-4xl font-bold tracking-tight text-[var(--color-heading,var(--color-text))] sm:text-5xl lg:text-6xl"
          style="font-family: var(--block-heading-family, var(--font-heading, var(--font-serif, ui-serif, Georgia, Cambria, 'Times New Roman', Times, serif))); font-size: var(--block-heading-size); font-weight: var(--block-heading-weight);"
        >
          {#if splitHeadline}
            {splitHeadline.before}<span class="text-[var(--color-success)]">{splitHeadline.match}</span>{splitHeadline.after}
          {:else}
            {props.headline}
          {/if}
        </h1>
      {/if}
      {#if props.tagline}
        <p class="mt-4 text-balance text-lg font-semibold text-[var(--color-text)] sm:text-xl">
          {props.tagline}
        </p>
      {/if}
      {#if props.subhead}
        <p class="mt-6 text-balance text-lg text-[var(--color-text-subtle)] sm:text-xl" style="font-family: var(--block-body-family); font-size: var(--block-body-size); font-weight: var(--block-body-weight);">
          {props.subhead}
        </p>
      {/if}
      {#if props.ctas && props.ctas.length > 0}
        <div class="mt-10 flex flex-wrap items-center justify-center gap-4">
          {#each props.ctas as cta (cta.href + cta.label)}
            <a
              href={cta.href}
              class="inline-flex items-center justify-center rounded-[var(--radius)] px-6 py-3 text-sm font-semibold transition-all
                {cta.variant === 'primary'
                  ? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] hover:shadow-lg'
                  : cta.variant === 'outline'
                    ? 'border border-[var(--color-border)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'
                    : 'text-[var(--color-primary)] hover:underline'
                }"
            >
              {cta.label}
            </a>
          {/each}
        </div>
      {/if}
      {#if props.stats && props.stats.length > 0}
        <!-- Mobile: tighter gap/size so three stats fit a 320px row. -->
        <div class="mt-16 grid grid-cols-3 gap-4 sm:gap-8">
          {#each props.stats as stat}
            <div>
              <p class="text-2xl font-bold text-[var(--color-text)] sm:text-3xl">{stat.value}</p>
              <p class="mt-1 text-xs text-[var(--color-muted)] sm:text-sm">{stat.label}</p>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  </section>
{/if}

<style>
  /* Split hero: theme-aware card. Light keeps the original blue→green tint;
     dark falls back to a surface-toned gradient so the card never stays light. */
  .hero-split-card {
    background: linear-gradient(to bottom right, #f4f8ff, #e8f5ee);
  }
  :global([data-theme='dark']) .hero-split-card {
    background: linear-gradient(to bottom right, #142033, #0d1524);
  }

  /* Media edge fade — blends the media into the card bg. Mobile fades
     downward (media stacked above text); lg+ fades right-to-left off the
     media's leading edge (media anchored right, text on the left). */
  .hero-media-fade {
    background: linear-gradient(to bottom, transparent, transparent, rgb(255 255 255 / 0.15));
  }
  @media (min-width: 1024px) {
    .hero-media-fade {
      background: linear-gradient(to right, #f4f8ff, rgb(244 248 255 / 0.4), transparent);
    }
  }
  /* Dark: invert+rereflect baked image media so the illustration tracks the
     theme — its light background becomes dark while hues are preserved. SVG
     media (GuestHeroIllustration) already reads theme vars, so it's untouched. */
  :global([data-theme='dark']) .hero-split-card img {
    filter: invert(0.85) hue-rotate(180deg);
  }
  :global([data-theme='dark']) .hero-media-fade {
    background: linear-gradient(to bottom, transparent, transparent, rgb(13 21 36 / 0.3));
  }
  @media (min-width: 1024px) {
    :global([data-theme='dark']) .hero-media-fade {
      background: linear-gradient(to right, #142033, rgb(20 32 51 / 0.4), transparent);
    }
  }

  .hero-eyebrow {
    color: #2563eb;
  }
  :global([data-theme='dark']) .hero-eyebrow {
    color: #93c5fd;
  }

  .hero-heading {
    color: #0f1e4e;
  }
  :global([data-theme='dark']) .hero-heading {
    color: var(--color-text);
  }

  .hero-arrow {
    background: rgb(255 255 255 / 0.9);
    color: #1e293b;
  }
  .hero-arrow:hover {
    background: #ffffff;
  }
  :global([data-theme='dark']) .hero-arrow {
    background: rgb(30 41 59 / 0.9);
    color: #f1f5f9;
  }
  :global([data-theme='dark']) .hero-arrow:hover {
    background: #1e293b;
  }
</style>

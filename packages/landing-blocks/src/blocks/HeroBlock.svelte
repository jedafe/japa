<script lang="ts">
  import { onMount } from 'svelte';
  import type { CTA, HeroProps } from '../types';
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

  // CTA variants share the EntityHero button trio so page-builder CTAs read
  // like the /tools hero actions (accent primary, bordered secondary,
  // quiet ghost/link).
  function ctaClass(variant: CTA['variant']): string {
    const base =
      'inline-flex items-center justify-center gap-2 rounded-[var(--radius)] px-5 py-2.5 text-sm font-semibold transition-all duration-200';
    switch (variant) {
      case 'outline':
        return `${base} border border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]`;
      case 'ghost':
        return `${base} text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]`;
      case 'link':
        return `inline-flex items-center gap-1 px-1 py-2.5 text-sm font-semibold text-[var(--hero-accent)] transition-opacity hover:opacity-75`;
      default:
        return `${base} bg-[var(--hero-accent)] text-[var(--hero-contrast)] shadow-sm hover:bg-[color-mix(in_srgb,var(--hero-accent)_85%,var(--hero-contrast))]`;
    }
  }
</script>

<!-- Moderate hero panel — same visual family as the /tools/{slug} hero:
     accent-tinted rounded panel holding a big bold title, subtitle, CTA row
     and a right-side artwork region (stacked above the copy on mobile). -->
<section class="px-6 py-4">
  <div class="pb-hero relative mx-auto overflow-hidden rounded-[var(--radius-card)] border {layout === 'split' ? 'max-w-6xl' : 'max-w-4xl'}">
    {#if layout === 'split'}
      <div class="relative grid items-center gap-5 px-5 py-4 sm:px-7 sm:py-5 lg:grid-cols-[minmax(0,1fr)_minmax(0,240px)]">
        <div class="min-w-0">
          {#if props.eyebrow}
            <p class="pb-hero-eyebrow text-xs font-semibold uppercase tracking-wide">
              {props.eyebrow}
            </p>
          {/if}

          <h1
            class="mt-1.5 text-balance text-2xl font-bold tracking-tight text-[var(--color-text)] sm:text-3xl"
            style="font-family: var(--block-heading-family, var(--font-heading, var(--font-sans))); font-size: var(--block-heading-size); font-weight: var(--block-heading-weight);"
          >
            {#if splitHeadline}
              {splitHeadline.before}<span class="text-[var(--color-success)]">{splitHeadline.match}</span>{splitHeadline.after}
            {:else}
              {props.headline}
            {/if}
          </h1>

          {#if props.tagline}
            <p class="mt-2 text-balance text-sm font-semibold text-[var(--color-text)] sm:text-base">
              {props.tagline}
            </p>
          {/if}

          {#if props.subhead}
            <p
              class="mt-2 max-w-lg text-balance text-sm text-[var(--color-text-subtle)]"
              style="font-family: var(--block-body-family); font-size: var(--block-body-size); font-weight: var(--block-body-weight);"
            >
              {props.subhead}
            </p>
          {/if}

          {#if props.ctas && props.ctas.length > 0}
            <div class="mt-4 flex flex-wrap items-center gap-3">
              {#each props.ctas as cta (cta.href + cta.label)}
                <a href={cta.href} class={ctaClass(cta.variant)}>
                  {cta.label}
                </a>
              {/each}
            </div>
          {/if}

          {#if props.trust_text}
            <p class="mt-3 text-xs text-[var(--color-muted)]">{props.trust_text}</p>
          {/if}
        </div>

        <!-- Artwork — right column on lg+, stacked above the copy below that.
             pointer-events-none on the wrapper keeps the decorative media from
             swallowing clicks; slider controls re-enable their own events. -->
        <div class="relative order-first h-32 sm:h-36 lg:order-none lg:h-40" aria-hidden={!props.media && !props.slides?.length ? 'true' : undefined}>
          {#if hasSlider}
            <div class="pointer-events-none h-full w-full overflow-hidden">
              <div
                class="flex h-full w-full transition-transform duration-500 motion-reduce:transition-none"
                style="transform: translateX(-{activeIndex * 100}%)"
              >
                {#each props.slides ?? [] as slide, i (slide.url + i)}
                  <div class="relative h-full w-full shrink-0">
                    <img
                      src={slide.url}
                      alt={slide.alt}
                      class="pb-hero-img h-full w-full object-contain"
                      loading={i === 0 ? 'eager' : 'lazy'}
                    />
                  </div>
                {/each}
              </div>
            </div>

            <!-- Prev / Next -->
            <div class="absolute inset-y-0 start-0 z-20 flex items-center ps-1">
              <button
                type="button"
                onclick={prev}
                aria-label="Previous slide"
                class="hero-arrow flex h-9 w-9 items-center justify-center rounded-full shadow-md backdrop-blur-sm transition"
              >
                <ChevronLeft class="rtl:-scale-x-100" size={18} />
              </button>
            </div>
            <div class="absolute inset-y-0 end-0 z-20 flex items-center pe-1">
              <button
                type="button"
                onclick={next}
                aria-label="Next slide"
                class="hero-arrow flex h-9 w-9 items-center justify-center rounded-full shadow-md backdrop-blur-sm transition"
              >
                <ChevronRight class="rtl:-scale-x-100" size={18} />
              </button>
            </div>

            <!-- Dots -->
            <div class="absolute bottom-2 end-3 z-20 flex gap-2">
              {#each props.slides ?? [] as _, i}
                <button
                  type="button"
                  onclick={() => goTo(i)}
                  aria-label="Go to slide {i + 1}"
                  aria-current={i === activeIndex}
                  class="h-2 w-2 rounded-full transition {i === activeIndex ? 'scale-110 bg-[var(--hero-accent)]' : 'bg-[var(--color-surface)]/80 hover:bg-[var(--color-surface)]'}"
                ></button>
              {/each}
            </div>
          {:else if props.media}
            <img
              src={props.media.url}
              alt={props.media.alt}
              class="pb-hero-img h-full w-full object-contain"
              loading="eager"
            />
          {:else}
            <div class="pointer-events-none h-full w-full" aria-hidden="true">
              <GuestHeroIllustration />
            </div>
          {/if}
        </div>

        <!-- Stats — compact strip spanning the full panel width so they
             don't stretch the copy column vertically. -->
        {#if props.stats && props.stats.length > 0}
          <div class="flex flex-wrap items-center gap-x-6 gap-y-2 border-t border-[var(--color-border)]/60 pt-3 lg:col-span-2">
            {#each props.stats as stat}
              <div>
                <p class="text-base font-bold leading-tight text-[var(--color-text)]">{stat.value}</p>
                <p class="mt-0.5 text-xs text-[var(--color-muted)]">{stat.label}</p>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    {:else}
      <div class="relative mx-auto max-w-2xl px-5 py-4 text-center sm:px-7 sm:py-5">
        {#if props.eyebrow}
          <p class="pb-hero-eyebrow text-xs font-semibold uppercase tracking-wide">
            {props.eyebrow}
          </p>
        {/if}
        {#if props.headline}
          <h1
            class="mt-1.5 text-balance text-2xl font-bold tracking-tight text-[var(--color-text)] sm:text-3xl"
            style="font-family: var(--block-heading-family, var(--font-heading, var(--font-sans))); font-size: var(--block-heading-size); font-weight: var(--block-heading-weight);"
          >
            {#if splitHeadline}
              {splitHeadline.before}<span class="text-[var(--color-success)]">{splitHeadline.match}</span>{splitHeadline.after}
            {:else}
              {props.headline}
            {/if}
          </h1>
        {/if}
        {#if props.tagline}
          <p class="mt-2 text-balance text-sm font-semibold text-[var(--color-text)] sm:text-base">
            {props.tagline}
          </p>
        {/if}
        {#if props.subhead}
          <p
            class="mx-auto mt-2 max-w-lg text-balance text-sm text-[var(--color-text-subtle)]"
            style="font-family: var(--block-body-family); font-size: var(--block-body-size); font-weight: var(--block-body-weight);"
          >
            {props.subhead}
          </p>
        {/if}
        {#if props.ctas && props.ctas.length > 0}
          <div class="mt-4 flex flex-wrap items-center justify-center gap-3">
            {#each props.ctas as cta (cta.href + cta.label)}
              <a href={cta.href} class={ctaClass(cta.variant)}>
                {cta.label}
              </a>
            {/each}
          </div>
        {/if}
        {#if props.trust_text}
          <p class="mt-3 text-xs text-[var(--color-muted)]">{props.trust_text}</p>
        {/if}
        {#if props.stats && props.stats.length > 0}
          <div class="mt-4 flex flex-wrap items-center justify-center gap-x-6 gap-y-2">
            {#each props.stats as stat}
              <div>
                <p class="text-base font-bold leading-tight text-[var(--color-text)]">{stat.value}</p>
                <p class="mt-0.5 text-xs text-[var(--color-muted)]">{stat.label}</p>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    {/if}
  </div>
</section>

<style>
  /* Tinted panel — same token-driven gradient as the /tools ToolHeader so the
     hero follows the site palette (and dark mode) automatically. */
  .pb-hero {
    --hero-accent: var(--color-primary);
    --hero-contrast: var(--color-primary-contrast, #ffffff);
    border-color: color-mix(in srgb, var(--hero-accent) 18%, var(--color-border));
    background:
      radial-gradient(
        140% 200% at 96% 0%,
        color-mix(in srgb, var(--hero-accent) 14%, transparent),
        transparent 55%
      ),
      radial-gradient(
        110% 140% at -4% 112%,
        color-mix(in srgb, var(--hero-accent) 8%, transparent),
        transparent 55%
      ),
      linear-gradient(
        135deg,
        color-mix(in srgb, var(--hero-accent) 7%, var(--color-surface)) 0%,
        color-mix(in srgb, var(--hero-accent) 13%, var(--color-surface)) 100%
      );
  }

  .pb-hero-eyebrow {
    color: var(--hero-accent);
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

  /* Dark: invert+rereflect baked image media so the illustration tracks the
     theme — its light background becomes dark while hues are preserved. The
     SVG fallback (GuestHeroIllustration) reads theme vars, so it's untouched. */
  :global([data-theme='dark']) .pb-hero-img {
    filter: invert(0.85) hue-rotate(180deg);
  }
</style>

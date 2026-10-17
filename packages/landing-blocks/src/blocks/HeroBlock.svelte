<script lang="ts">
  import { onMount } from 'svelte';
  import type { Component } from 'svelte';
  import type { HeroProps } from '../types';
  import GuestHeroIllustration from './GuestHeroIllustration.svelte';
  import * as LucideIcons from 'lucide-svelte';

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

  const ChevronLeft = (LucideIcons as Record<string, unknown>)['ChevronLeft'] as Component | undefined;
  const ChevronRight = (LucideIcons as Record<string, unknown>)['ChevronRight'] as Component | undefined;
</script>

{#if layout === 'split'}
  <section class="relative px-6 py-12 lg:py-20">
    <div class="mx-auto max-w-7xl">
      <div class="relative h-[480px] w-full overflow-hidden rounded-[var(--radius-card)] bg-gradient-to-br from-[#f4f8ff] to-[#e8f5ee] shadow-xl sm:h-[520px] lg:h-[560px]">
        <!-- Background media / slider -->
        {#if hasSlider}
          <div
            class="absolute inset-0 flex h-full w-full transition-transform duration-500 motion-reduce:transition-none"
            style="transform: translateX(-{activeIndex * 100}%)"
          >
            {#each props.slides ?? [] as slide, i (slide.url + i)}
              <div class="relative h-full w-full shrink-0">
                <img
                  src={slide.url}
                  alt={slide.alt}
                  class="h-full w-full object-cover"
                  loading={i === 0 ? 'eager' : 'lazy'}
                />
                <div class="absolute inset-0 bg-gradient-to-r from-white/85 via-white/40 to-transparent"></div>
              </div>
            {/each}
          </div>
        {:else if props.media}
          <img
            src={props.media.url}
            alt={props.media.alt}
            class="absolute inset-0 h-full w-full object-cover"
            loading="eager"
          />
          <div class="absolute inset-0 bg-gradient-to-r from-white/85 via-white/40 to-transparent"></div>
        {:else}
          <GuestHeroIllustration />
          <div class="absolute inset-0 bg-gradient-to-r from-white/80 via-white/40 to-transparent"></div>
        {/if}

        <!-- Text content -->
        <div class="relative z-10 flex h-full flex-col justify-center px-8 sm:px-10 lg:px-16 max-w-xl lg:max-w-2xl">
          {#if props.eyebrow}
            <p class="mb-4 text-sm font-semibold uppercase tracking-wide text-[#2563eb]">
              {props.eyebrow}
            </p>
          {/if}

          <h1
            class="text-balance text-4xl font-bold tracking-tight text-[#0f1e4e] sm:text-5xl lg:text-6xl"
            style="font-family: var(--block-heading-family, var(--font-heading, var(--font-sans))); font-size: var(--block-heading-size); font-weight: var(--block-heading-weight);"
          >
            {#if splitHeadline}
              {splitHeadline.before}<span class="text-[#16a34a]">{splitHeadline.match}</span>{splitHeadline.after}
            {:else}
              {props.headline}
            {/if}
          </h1>

          {#if props.tagline}
            <p class="mt-4 text-balance text-lg font-medium text-[#0f1e4e] sm:text-xl">
              {props.tagline}
            </p>
          {/if}

          {#if props.subhead}
            <p
              class="mt-4 text-balance text-lg text-slate-700"
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
                      ? 'border-2 border-[#2563eb] text-[#2563eb] hover:bg-[#2563eb] hover:text-white'
                      : cta.variant === 'ghost'
                        ? 'text-[#0f1e4e] hover:bg-black/5'
                        : 'bg-gradient-to-r from-[#2563eb] to-[#16a34a] text-white shadow-md hover:shadow-lg hover:from-[#1d4ed8] hover:to-[#15803d]'
                    }"
                >
                  {cta.label}
                </a>
              {/each}
            </div>
          {/if}

          {#if props.trust_text}
            <p class="mt-5 text-sm text-slate-500">{props.trust_text}</p>
          {/if}
        </div>

        {#if hasSlider}
          <!-- Prev / Next arrows -->
          <div class="absolute inset-y-0 left-0 z-20 flex items-center pl-2 sm:pl-4">
            <button
              type="button"
              onclick={prev}
              aria-label="Previous slide"
              class="flex h-10 w-10 items-center justify-center rounded-full bg-white/90 text-slate-800 shadow-lg backdrop-blur-sm transition hover:bg-white"
            >
              {#if ChevronLeft}
                <ChevronLeft size={20} />
              {:else}
                <span aria-hidden="true">&#10094;</span>
              {/if}
            </button>
          </div>
          <div class="absolute inset-y-0 right-0 z-20 flex items-center pr-2 sm:pr-4">
            <button
              type="button"
              onclick={next}
              aria-label="Next slide"
              class="flex h-10 w-10 items-center justify-center rounded-full bg-white/90 text-slate-800 shadow-lg backdrop-blur-sm transition hover:bg-white"
            >
              {#if ChevronRight}
                <ChevronRight size={20} />
              {:else}
                <span aria-hidden="true">&#10095;</span>
              {/if}
            </button>
          </div>

          <!-- Dots -->
          <div class="absolute bottom-5 right-4 z-20 flex gap-2 sm:right-6 lg:right-8">
            {#each props.slides ?? [] as _, i}
              <button
                type="button"
                onclick={() => goTo(i)}
                aria-label="Go to slide {i + 1}"
                aria-current={i === activeIndex}
                class="h-2.5 w-2.5 rounded-full transition {i === activeIndex ? 'bg-[#2563eb] scale-110' : 'bg-white/80 hover:bg-white'}"
              ></button>
            {/each}
          </div>
        {/if}
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
        <div class="mt-16 grid grid-cols-3 gap-8">
          {#each props.stats as stat}
            <div>
              <p class="text-3xl font-bold text-[var(--color-text)]">{stat.value}</p>
              <p class="mt-1 text-sm text-[var(--color-muted)]">{stat.label}</p>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  </section>
{/if}

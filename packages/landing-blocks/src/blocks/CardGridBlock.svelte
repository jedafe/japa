<script lang="ts">
  import type { CardGridProps } from '../types';
  import type { Component } from 'svelte';
  import * as LucideIcons from 'lucide-svelte';

  interface Props {
    props: CardGridProps;
    id?: string;
  }
  let { props, id }: Props = $props();

  // Unique ID for this card-grid instance — used to scope responsive CSS.
  // Use the stable block id so SSR and client produce the same value (no hydration mismatch).
  const gridId = $derived(id ? `cg-${id}` : `cg-${crypto.randomUUID()}`);

  // Base desktop column count (clamped 1–6).
  const desktopCols = $derived(Math.max(1, Math.min(6, props.columns ?? 3)));

  // Generate responsive CSS for per-device column overrides.
  // Uses @container queries so they respond to the preview container width
  // (not the browser viewport), making them work in the editor preview too.
  // The desktop columns are set via inline style on the element. Inline styles
  // override normal stylesheet rules, so we use !important in the @container
  // rules to override the inline desktop value at tablet/mobile breakpoints.
  const responsiveCss = $derived.by(() => {
    const rc = props.responsive_columns;
    const tabletCols = rc?.tablet !== undefined ? Math.max(1, Math.min(6, rc.tablet)) : null;
    // Mobile defaults to 2 columns when the block sets no explicit override —
    // desktop column counts would otherwise squeeze onto phone viewports.
    const mobileCols = rc?.mobile !== undefined ? Math.max(1, Math.min(6, rc.mobile)) : Math.min(desktopCols, 2);

    const parts: string[] = [];
    const scope = `[data-cg="${gridId}"]`;
    if (tabletCols !== null) {
      parts.push(`@container (max-width: 1024px) { ${scope} { grid-template-columns: repeat(${tabletCols}, minmax(0, 1fr)) !important; } }`);
    }
    parts.push(`@container (max-width: 768px) { ${scope} { grid-template-columns: repeat(${mobileCols}, minmax(0, 1fr)) !important; } }`);
    return parts.join('\n');
  });

  // Resolve Lucide icon by name.
  function getIcon(name?: string): Component | null {
    if (!name) return null;
    const found = (LucideIcons as Record<string, unknown>)[name];
    return (found as Component) ?? null;
  }

  const ArrowRight = $derived((LucideIcons as Record<string, unknown>)['ArrowRight'] as Component | undefined);
</script>

<svelte:head>
  {@html responsiveCss ? '<style>' + responsiveCss + '<\/style>' : ''}
</svelte:head>

{#if props.card_variant === 'overlay'}
  <section class="px-6 py-10">
    <div class="mx-auto max-w-7xl">
      <div
        class="grid gap-5"
        data-cg={gridId}
        style="grid-template-columns: repeat({desktopCols}, minmax(0, 1fr))"
      >
        {#each props.cards as card, i (card.title)}
          {@const Icon = getIcon(card.icon)}
          {@const waveColor = card.icon_background_color ?? 'var(--color-primary)'}
          <a
            href={card.cta?.href ?? '#'}
            class="group flex flex-col overflow-hidden rounded-[var(--radius-card)] bg-white shadow-md transition-all hover:shadow-xl no-underline"
            style="--wave-color: {waveColor};"
          >
            <div class="p-6">
              {#if Icon}
                <div
                  class="mb-5 inline-flex h-12 w-12 items-center justify-center rounded-full shadow-sm"
                  style="background-color: {card.icon_background_color ?? 'var(--color-primary)'}; color: {card.icon_color ?? 'white'}"
                >
                  <Icon size={24} />
                </div>
              {/if}
              {#if card.title}
                <h3 class="text-2xl font-bold tracking-tight text-[#0f1e4e]">{card.title}</h3>
              {/if}
              {#if card.description}
                <p class="mt-2 text-sm leading-relaxed text-slate-600">{card.description}</p>
              {/if}
            </div>

            <div class="relative mt-auto h-64 w-full overflow-hidden">
              {#if card.media}
                <img
                  src={card.media.url}
                  alt={card.media.alt}
                  class="absolute inset-0 h-full w-full object-cover transition-transform duration-500 group-hover:scale-105"
                  loading="lazy"
                />
              {:else}
                <div class="h-full w-full bg-gradient-to-br from-[#f4f8ff] to-[#e8f5ee]"></div>
              {/if}

              <!-- Wave overlay -->
              <svg
                class="absolute -bottom-1 start-0 end-0 h-28 w-full"
                viewBox="0 0 400 120"
                preserveAspectRatio="none"
                aria-hidden="true"
              >
                <defs>
                  <linearGradient id="wg-{gridId}-{i}" x1="0" y1="0" x2="1" y2="0">
                    <stop offset="0%" stop-color={waveColor} />
                    <stop offset="100%" stop-color={waveColor === '#16a34a' ? '#2563eb' : '#16a34a'} />
                  </linearGradient>
                </defs>
                <path
                  d="M0,90 C60,20 180,130 400,30 L400,120 L0,120 Z"
                  fill="url(#wg-{gridId}-{i})"
                  opacity="0.92"
                />
              </svg>

              <!-- Brand mark + CTA on the wave -->
              <div class="absolute inset-x-0 bottom-0 z-10 flex items-center justify-between p-4">
                <div class="flex items-center gap-2">
                  <svg viewBox="0 0 32 32" class="h-6 w-6 shrink-0">
                    <circle cx="16" cy="16" r="14" fill="var(--wave-color)" />
                    <path
                      d="M 21 8 v 10 c 0 4 -2 6 -6 6 s -4 -2 -4 -2"
                      stroke="white"
                      stroke-width="3"
                      fill="none"
                      stroke-linecap="round"
                    />
                  </svg>
                  <span class="text-sm font-semibold text-white drop-shadow-sm">
                    Japa <span style="color: {card.icon_background_color === '#16a34a' ? '#bbf7d0' : '#bfdbfe'}">Connect</span>
                  </span>
                </div>
                {#if card.cta}
                  <span
                    class="inline-flex shrink-0 items-center gap-1 rounded-full border border-white/60 bg-white px-3 py-1.5 text-xs font-semibold transition group-hover:bg-white/90"
                    style="color: var(--wave-color)"
                  >
                    {card.cta.label}
                    {#if ArrowRight}
                      <ArrowRight class="rtl:-scale-x-100" size={14} />
                    {:else}
                      <span aria-hidden="true">&#8594;</span>
                    {/if}
                  </span>
                {/if}
              </div>
            </div>
          </a>
        {/each}
      </div>
    </div>
  </section>
{:else}
  <section class="px-6 py-16">
    <div class="mx-auto max-w-6xl">
      <div
        class="grid gap-6"
        data-cg={gridId}
        style="grid-template-columns: repeat({desktopCols}, minmax(0, 1fr))"
      >
        {#each props.cards as card}
          <div class="group rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 transition-all hover:shadow-lg hover:-translate-y-1">
            {#if card.media}
              <img src={card.media.url} alt={card.media.alt} class="mb-4 w-full rounded-[var(--radius)] object-cover" loading="lazy" />
            {/if}
            {#if card.icon}
              {@const Icon = getIcon(card.icon)}
              {#if Icon}
                <div class="mb-4 inline-flex rounded-[var(--radius)] bg-[var(--color-surface-subtle)] p-3 text-[var(--color-primary)]">
                  <Icon size={24} />
                </div>
              {/if}
            {/if}
            {#if card.stat_value}
              <p class="text-3xl font-bold text-[var(--color-text)]">{card.stat_value}</p>
              {#if card.stat_label}
                <p class="mt-1 text-sm text-[var(--color-muted)]">{card.stat_label}</p>
              {/if}
            {/if}
            {#if card.title}
              <h3 class="text-lg font-semibold text-[var(--color-text)]">{card.title}</h3>
            {/if}
            {#if card.description}
              <p class="mt-2 text-sm text-[var(--color-text-subtle)]">{card.description}</p>
            {/if}
            {#if card.rating}
              <div class="mt-3 flex gap-1">
                {#each Array(Math.round(card.rating)) as _}
                  <span class="text-[var(--color-warning)]">&#9733;</span>
                {/each}
              </div>
            {/if}
            {#if card.cta}
              <a href={card.cta.href} class="mt-4 inline-flex items-center gap-1 text-sm font-semibold text-[var(--color-primary)] hover:underline">
                {card.cta.label}
                &#8594;
              </a>
            {/if}
          </div>
        {/each}
      </div>
    </div>
  </section>
{/if}

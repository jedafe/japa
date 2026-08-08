<script lang="ts">
  import type { CardGridProps } from '../types';
  import type { Component } from 'svelte';
  import * as LucideIcons from 'lucide-svelte';

  interface Props {
    props: CardGridProps;
  }
  let { props }: Props = $props();

  // Unique ID for this card-grid instance — used to scope responsive CSS.
  const gridId = `cg-${crypto.randomUUID()}`;

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
    const mobileCols = rc?.mobile !== undefined ? Math.max(1, Math.min(6, rc.mobile)) : null;
    if (tabletCols === null && mobileCols === null) return null;

    const parts: string[] = [];
    const scope = `[data-cg="${gridId}"]`;
    if (tabletCols !== null) {
      parts.push(`@container (max-width: 1024px) { ${scope} { grid-template-columns: repeat(${tabletCols}, minmax(0, 1fr)) !important; } }`);
    }
    if (mobileCols !== null) {
      parts.push(`@container (max-width: 768px) { ${scope} { grid-template-columns: repeat(${mobileCols}, minmax(0, 1fr)) !important; } }`);
    }
    return parts.length > 0 ? parts.join('\n') : null;
  });

  // Resolve Lucide icon by name.
  function getIcon(name?: string): Component | null {
    if (!name) return null;
    const found = (LucideIcons as Record<string, unknown>)[name];
    return (found as Component) ?? null;
  }
</script>

<svelte:head>
  {@html responsiveCss ? '<style>' + responsiveCss + '<\/style>' : ''}
</svelte:head>

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

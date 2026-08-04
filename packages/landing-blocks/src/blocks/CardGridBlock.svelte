<script lang="ts">
  import type { CardGridProps } from '../types';
  import type { Component } from 'svelte';
  import * as LucideIcons from 'lucide-svelte';

  interface Props {
    props: CardGridProps;
  }
  let { props }: Props = $props();

  const colsClass = $derived(
    props.columns === 1 ? 'grid-cols-1' :
    props.columns === 2 ? 'grid-cols-1 sm:grid-cols-2' :
    props.columns === 3 ? 'grid-cols-1 sm:grid-cols-2 lg:grid-cols-3' :
    props.columns === 4 ? 'grid-cols-1 sm:grid-cols-2 lg:grid-cols-4' :
    'grid-cols-1 sm:grid-cols-2 lg:grid-cols-3'
  );

  // Resolve Lucide icon by name.
  function getIcon(name?: string): Component | null {
    if (!name) return null;
    const found = (LucideIcons as Record<string, unknown>)[name];
    return (found as Component) ?? null;
  }
</script>

<section class="px-6 py-16">
  <div class="mx-auto max-w-6xl">
    <div class="grid {colsClass} gap-6">
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

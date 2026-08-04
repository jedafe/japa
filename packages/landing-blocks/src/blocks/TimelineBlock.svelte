<script lang="ts">
  import type { TimelineProps } from '../types';
  import type { Component } from 'svelte';
  import * as LucideIcons from 'lucide-svelte';

  interface Props {
    props: TimelineProps;
  }
  let { props }: Props = $props();

  function getIcon(name?: string): Component | null {
    if (!name) return null;
    const found = (LucideIcons as Record<string, unknown>)[name];
    return (found as Component) ?? null;
  }
</script>

<section class="px-6 py-16">
  <div class="mx-auto max-w-4xl">
    {#if props.layout === 'horizontal'}
      <div class="relative">
        <div class="absolute top-8 left-0 right-0 h-0.5 bg-[var(--color-border)]"></div>
        <div class="relative grid gap-4" style={`grid-template-columns: repeat(${props.steps.length}, 1fr)`}>
          {#each props.steps as step, i}
            <div class="flex flex-col items-center text-center">
              <div class="z-10 flex h-16 w-16 items-center justify-center rounded-full border-2 border-[var(--color-primary)] bg-[var(--color-surface)]">
                {#if step.icon}
                  {@const Icon = getIcon(step.icon)}
                  {#if Icon}
                    <Icon size={24} />
                  {/if}
                {:else}
                  <span class="text-sm font-bold text-[var(--color-primary)]">0{i + 1}</span>
                {/if}
              </div>
              <h3 class="mt-3 text-sm font-semibold text-[var(--color-text)]">{step.title}</h3>
              {#if step.description}
                <p class="mt-1 text-xs text-[var(--color-text-subtle)]">{step.description}</p>
              {/if}
            </div>
          {/each}
        </div>
      </div>
    {:else if props.layout === 'zigzag'}
      <div class="space-y-8">
        {#each props.steps as step, i}
          <div class="flex gap-6 {i % 2 === 0 ? '' : 'flex-row-reverse text-right'}">
            <div class="flex-shrink-0 flex h-12 w-12 items-center justify-center rounded-full bg-[var(--color-primary)] text-[var(--color-primary-contrast)] font-bold text-sm">
              {i + 1}
            </div>
            <div class="flex-1 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6">
              {#if step.media}
                <img src={step.media.url} alt={step.media.alt} class="mb-3 w-full rounded-[var(--radius)] object-cover" loading="lazy" />
              {/if}
              <h3 class="text-lg font-semibold text-[var(--color-text)]">{step.title}</h3>
              {#if step.description}
                <p class="mt-2 text-sm text-[var(--color-text-subtle)]">{step.description}</p>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    {:else}
      <!-- vertical -->
      <div class="relative pl-8">
        <div class="absolute left-4 top-0 bottom-0 w-0.5 bg-[var(--color-border)]"></div>
        <div class="space-y-8">
          {#each props.steps as step, i}
            <div class="relative">
              <div class="absolute -left-8 flex h-8 w-8 items-center justify-center rounded-full border-2 border-[var(--color-primary)] bg-[var(--color-surface)]">
                <span class="text-xs font-bold text-[var(--color-primary)]">{i + 1}</span>
              </div>
              <div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6">
                {#if step.media}
                  <img src={step.media.url} alt={step.media.alt} class="mb-3 w-full rounded-[var(--radius)] object-cover" loading="lazy" />
                {/if}
                <h3 class="text-lg font-semibold text-[var(--color-text)]">{step.title}</h3>
                {#if step.description}
                  <p class="mt-2 text-sm text-[var(--color-text-subtle)]">{step.description}</p>
                {/if}
              </div>
            </div>
          {/each}
        </div>
      </div>
    {/if}
  </div>
</section>

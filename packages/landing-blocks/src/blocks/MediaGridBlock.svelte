<script lang="ts">
  import type { MediaGridProps } from '../types';

  interface Props {
    props: MediaGridProps;
  }
  let { props }: Props = $props();

  const colsClass = $derived(
    props.columns <= 4 ? `grid-cols-2 sm:grid-cols-${props.columns}` :
    `grid-cols-2 sm:grid-cols-4 lg:grid-cols-${props.columns}`
  );

  const isLogo = $derived(props.card_style === 'logo');
</script>

<section class="px-6 py-16">
  <div class="mx-auto max-w-6xl">
    <div class="grid {colsClass} gap-6">
      {#each props.items as item}
        {#if isLogo}
          <a
            href={item.href ?? '#'}
            class="flex items-center justify-center rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-8 grayscale transition-all hover:grayscale-0 hover:shadow-md"
          >
            <img src={item.media.url} alt={item.media.alt} class="max-h-16 w-auto object-contain" loading="lazy" />
          </a>
        {:else}
          <div class="overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
            <img src={item.media.url} alt={item.media.alt} class="w-full object-cover" loading="lazy" />
            {#if item.caption}
              <p class="p-3 text-sm text-[var(--color-text-subtle)]">{item.caption}</p>
            {/if}
          </div>
        {/if}
      {/each}
    </div>
  </div>
</section>

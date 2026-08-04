<script lang="ts">
  import type { HeroProps } from '../types';

  interface Props {
    props: HeroProps;
  }
  let { props }: Props = $props();
</script>

<section class="relative flex min-h-[480px] flex-col items-center justify-center px-6 py-20 text-center">
  <div class="relative z-10 mx-auto max-w-4xl">
    {#if props.eyebrow}
      <p class="mb-4 text-sm font-semibold uppercase tracking-wide text-[var(--color-primary)]">
        {props.eyebrow}
      </p>
    {/if}
    {#if props.headline}
      <h1
        class="text-balance text-4xl font-bold tracking-tight text-[var(--color-text)] sm:text-5xl lg:text-6xl"
        style="font-family: var(--font-serif);"
      >
        {props.headline}
      </h1>
    {/if}
    {#if props.subhead}
      <p class="mt-6 text-balance text-lg text-[var(--color-text-subtle)] sm:text-xl">
        {props.subhead}
      </p>
    {/if}
    {#if props.ctas && props.ctas.length > 0}
      <div class="mt-10 flex flex-wrap items-center justify-center gap-4">
        {#each props.ctas as cta}
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

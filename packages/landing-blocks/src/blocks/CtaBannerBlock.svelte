<script lang="ts">
  import type { CtaBannerProps } from '../types';

  interface Props {
    props: CtaBannerProps;
  }
  let { props }: Props = $props();

  const align = $derived(props.alignment ?? 'center');
  const alignClass = $derived(
    align === 'left' ? 'text-left items-start' : align === 'right' ? 'text-right items-end' : 'text-center items-center'
  );
</script>

<section class="px-6 py-16">
  <div class="mx-auto max-w-4xl rounded-[var(--radius-card)] bg-gradient-to-br from-[var(--color-primary)] to-[var(--color-primary-hover)] p-12 text-[var(--color-primary-contrast)]">
    <div class="flex flex-col gap-6 {alignClass}">
      {#if props.headline}
        <h2 class="text-3xl font-bold tracking-tight sm:text-4xl">{props.headline}</h2>
      {/if}
      {#if props.subhead}
        <p class="text-lg opacity-90">{props.subhead}</p>
      {/if}
      {#if props.ctas && props.ctas.length > 0}
        <div class="flex flex-wrap gap-4 {align === 'center' ? 'justify-center' : align === 'right' ? 'justify-end' : ''}">
          {#each props.ctas as cta}
            <a
              href={cta.href}
              class="inline-flex items-center justify-center rounded-[var(--radius)] bg-[var(--color-primary-contrast)] px-6 py-3 text-sm font-semibold text-[var(--color-primary)] transition-all hover:shadow-lg"
            >
              {cta.label}
            </a>
          {/each}
        </div>
      {/if}
    </div>
  </div>
</section>

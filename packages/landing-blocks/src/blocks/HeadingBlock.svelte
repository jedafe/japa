<script lang="ts">
  import type { HeadingProps } from '../types';
  import LazyIcon from '../lib/LazyIcon.svelte';

  interface Props {
    props: HeadingProps;
  }
  let { props }: Props = $props();

  const sizeClass: Record<string, string> = {
    'xs': 'text-lg',
    'sm': 'text-xl',
    'md': 'text-2xl',
    'lg': 'text-3xl',
    'xl': 'text-4xl',
    '2xl': 'text-5xl',
    '3xl': 'text-6xl',
    '4xl': 'text-7xl',
  };

  const alignClass: Record<string, string> = {
    'left': 'text-start items-start',
    'center': 'text-center items-center',
    'right': 'text-end items-end',
  };

  const maxWidthClass: Record<string, string> = {
    'sm': 'max-w-md',
    'md': 'max-w-lg',
    'lg': 'max-w-2xl',
    'xl': 'max-w-4xl',
    'full': 'max-w-full',
  };

  const level = $derived(props.level ?? 'h2');
  const headingStyle = $derived(
    (props.accent === 'underline' && props.accent_color ? `border-color: ${props.accent_color};` : '') +
    (props.accent === 'gradient' && props.accent_color
      ? `background: linear-gradient(135deg, ${props.accent_color}, var(--color-primary)); -webkit-background-clip: text; -webkit-text-fill-color: transparent; background-clip: text;`
      : '') +
    (props.accent === 'highlight' && props.accent_color ? `background: ${props.accent_color}; padding: 0 0.2em; border-radius: 0.2em;` : '')
  );
</script>

<section class="px-6 py-12">
  <div class="mx-auto max-w-6xl flex flex-col {alignClass[props.align ?? 'left'] ?? 'text-start items-start'}">
    <div class="{maxWidthClass[props.max_width ?? 'xl'] ?? 'max-w-4xl'} w-full">
      {#if props.eyebrow || props.icon}
        <div class="mb-3 flex items-center gap-2 {props.align === 'center' ? 'justify-center' : ''}">
          {#if props.icon}
            <LazyIcon name={props.icon} size={16} class="text-[var(--color-primary)]" />
          {/if}
          {#if props.eyebrow}
            <span class="text-sm font-semibold uppercase tracking-wider text-[var(--color-primary)]">
              {props.eyebrow}
            </span>
          {/if}
        </div>
      {/if}

      <svelte:element
        this={level}
        class="font-bold tracking-tight text-[var(--color-heading,var(--color-text))] {sizeClass[props.size ?? 'xl'] ?? 'text-4xl'} {props.accent === 'underline' ? 'border-b-2 pb-2' : ''}"
        style="{headingStyle} font-family: var(--block-heading-family, var(--font-heading, var(--font-serif, ui-serif, Georgia, Cambria, 'Times New Roman', Times, serif))); font-size: var(--block-heading-size); font-weight: var(--block-heading-weight);"
      >
        {props.text}
      </svelte:element>

      {#if props.subheading}
        <p class="mt-3 text-base text-[var(--color-text-subtle)] sm:text-lg" style="font-family: var(--block-body-family); font-size: var(--block-body-size); font-weight: var(--block-body-weight);">
          {props.subheading}
        </p>
      {/if}

      {#if props.cta}
        <div class="mt-4 {props.align === 'center' ? 'flex justify-center' : ''}">
          <a
            href={props.cta.href}
            class="inline-flex items-center gap-1 text-sm font-semibold text-[var(--color-primary)] hover:underline"
          >
            {props.cta.label}
            <span aria-hidden="true">&#8594;</span>
          </a>
        </div>
      {/if}
    </div>
  </div>
</section>

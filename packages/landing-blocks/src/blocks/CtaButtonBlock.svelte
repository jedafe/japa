<script lang="ts">
  import type { CtaButtonProps } from '../types';
  import LazyIcon from '../lib/LazyIcon.svelte';

  interface Props {
    props: CtaButtonProps;
  }
  let { props }: Props = $props();

  const sizeClass: Record<string, string> = {
    'sm': 'px-4 py-2 text-sm',
    'md': 'px-5 py-2.5 text-sm',
    'lg': 'px-6 py-3 text-base',
    'xl': 'px-8 py-4 text-lg',
  };

  const variantClass: Record<string, string> = {
    'primary': 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] shadow-sm hover:shadow-md',
    'outline': 'border-2 border-[var(--color-primary)] text-[var(--color-primary)] hover:bg-[var(--color-primary)] hover:text-[var(--color-primary-contrast)]',
    'ghost': 'text-[var(--color-primary)] hover:bg-[var(--color-surface-subtle)]',
    'link': 'text-[var(--color-primary)] hover:underline px-0 py-1',
    'gradient': 'text-white shadow-sm hover:shadow-md hover:brightness-110',
  };

  const alignClass: Record<string, string> = {
    'left': 'justify-start',
    'center': 'justify-center',
    'right': 'justify-end',
  };

  const gradientStyle = $derived(
    props.variant === 'gradient'
      ? `background: linear-gradient(135deg, ${props.gradient_from ?? 'var(--color-primary)'}, ${props.gradient_to ?? '#a855f7'})`
      : undefined
  );
</script>

<section class="px-6 py-8">
  <div class="mx-auto max-w-6xl flex flex-col {alignClass[props.align ?? 'center'] ?? 'justify-center'}">
    <a
      href={props.href}
      target={props.new_tab ? '_blank' : undefined}
      rel={props.new_tab ? 'noopener noreferrer' : undefined}
      class="inline-flex items-center justify-center gap-2 rounded-[var(--radius)] font-semibold transition-all duration-200 {sizeClass[props.size ?? 'lg'] ?? 'px-6 py-3 text-base'} {variantClass[props.variant ?? 'primary'] ?? ''} {props.full_width ? 'w-full' : ''}"
      style={gradientStyle}
    >
      {#if props.icon}
        <LazyIcon name={props.icon} size={props.size === 'xl' ? 22 : props.size === 'lg' ? 20 : 18} />
      {/if}
      {props.label}
      {#if props.icon_right}
        <LazyIcon name={props.icon_right} size={props.size === 'xl' ? 22 : props.size === 'lg' ? 20 : 18} />
      {/if}
    </a>
    {#if props.subtext}
      <p class="mt-3 text-sm text-[var(--color-muted)] {alignClass[props.align ?? 'center'] ?? 'text-center'}">
        {props.subtext}
      </p>
    {/if}
  </div>
</section>

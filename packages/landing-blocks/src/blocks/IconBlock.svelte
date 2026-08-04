<script lang="ts">
  import type { IconProps } from '../types';
  import type { Component } from 'svelte';
  import * as LucideIcons from 'lucide-svelte';

  interface Props {
    props: IconProps;
  }
  let { props }: Props = $props();

  const sizeMap: Record<string, number> = { xs: 16, sm: 20, md: 24, lg: 32, xl: 48, '2xl': 64 };

  const pxSize = $derived(
    typeof props.size === 'number' ? props.size : sizeMap[props.size] ?? 24
  );

  const shapeClass = $derived(
    props.shape === 'circle' ? 'rounded-full' :
    props.shape === 'square' ? 'rounded-none' :
    props.shape === 'rounded' ? 'rounded-[var(--radius)]' :
    ''
  );

  const padding = $derived(pxSize >= 48 ? 'p-4' : pxSize >= 32 ? 'p-3' : 'p-2');

  function getIcon(name: string): Component | null {
    const found = (LucideIcons as Record<string, unknown>)[name];
    return (found as Component) ?? null;
  }
</script>

<section class="px-6 py-8">
  <div class="mx-auto max-w-6xl flex items-center justify-center gap-3">
    {@const Icon = getIcon(props.icon)}
    {#if props.href}
      <a href={props.href} class="inline-flex items-center gap-3">
        {#if Icon}
          <div class="inline-flex {shapeClass} {padding}" style="background-color: {props.background_color}; color: {props.color}">
            <Icon size={pxSize} />
          </div>
        {/if}
        {#if props.label}
          <span class="text-sm font-medium" style="color: {props.color}">{props.label}</span>
        {/if}
      </a>
    {:else}
      {#if Icon}
        <div class="inline-flex {shapeClass} {padding}" style="background-color: {props.background_color}; color: {props.color}">
          <Icon size={pxSize} />
        </div>
      {/if}
      {#if props.label}
        <span class="text-sm font-medium" style="color: {props.color}">{props.label}</span>
      {/if}
    {/if}
  </div>
</section>

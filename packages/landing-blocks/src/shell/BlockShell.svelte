<script lang="ts">
  import type { BlockStyle, MotionConfig, BackgroundConfig } from '../types';
  import { styleToCss } from './styleToCss';
  import Reveal from './Reveal.svelte';

  interface Props {
    style?: BlockStyle;
    motion?: MotionConfig;
    background?: BackgroundConfig;
    children: import('svelte').Snippet;
  }
  let { style, motion, background, children }: Props = $props();

  const resolved = $derived(styleToCss(style));
  const inlineStyle = $derived(
    Object.entries(resolved.inline)
      .map(([k, v]) => `${k}: ${v}`)
      .join('; ')
  );

  // Background rendering
  const hasBg = $derived(!!background && background.type !== 'none');
  const bgOverlay = $derived(background?.image?.overlay);
</script>

<Reveal {motion}>
  <div class="relative {resolved.classes}" style={inlineStyle || undefined}>
    {#if hasBg && background?.type === 'image' && background.image}
      <img
        src={background.image.url}
        alt={background.image.alt}
        class="absolute inset-0 h-full w-full {background.image.fit === 'contain' ? 'object-contain' : 'object-cover'}"
        loading="lazy"
      />
      {#if bgOverlay}
        <div class="absolute inset-0" style="background-color: {bgOverlay.color}; opacity: {bgOverlay.opacity / 100}"></div>
      {/if}
    {:else if hasBg && background?.type === 'gradient' && background.gradient}
      <div
        class="absolute inset-0"
        style="background: linear-gradient({background.gradient.angle === 'radial' ? 'radial' : `${background.gradient.angle}deg`}, {background.gradient.from}, {background.gradient.to})"
      ></div>
    {:else if hasBg && background?.type === 'color' && background.color}
      <div class="absolute inset-0" style="background-color: {background.color}"></div>
    {/if}
    <div class="relative" style={hasBg ? 'z-index: 10;' : undefined}>
      {@render children()}
    </div>
  </div>
</Reveal>

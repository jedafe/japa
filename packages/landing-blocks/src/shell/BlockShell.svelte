<script lang="ts">
  import type { BlockStyle, MotionConfig, BackgroundConfig } from '../types';
  import { styleToCss, responsiveStyleToCss } from './styleToCss';
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

  // Custom CSS + responsive overrides — both use a unique scoped selector.
  // Each BlockShell instance gets its own data attribute so CSS doesn't
  // leak to sibling blocks. The ID is generated once per instance.
  const scopeId = `bs-${crypto.randomUUID()}`;
  const scopeSelector = `[data-bs="${scopeId}"]`;
  const scopedCss = $derived.by(() => {
    const raw = style?.custom_css;
    if (!raw?.trim()) return null;
    // Replace `&` with the scoped selector `[data-bs="<id>"]`
    return raw.replace(/&/g, scopeSelector);
  });
  // Responsive container-query CSS from style.responsive overrides.
  const responsiveCss = $derived(responsiveStyleToCss(style, scopeSelector));
</script>

<svelte:head>
  {@html scopedCss ? '<style>' + scopedCss + '<\/style>' : ''}
  {@html responsiveCss ? '<style>' + responsiveCss + '<\/style>' : ''}
</svelte:head>

<Reveal {motion}>
  <div class="relative {resolved.classes}" data-bs={scopeId} style={inlineStyle || undefined}>
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

<script lang="ts">
  import type { BlockStyle, MotionConfig, BackgroundConfig } from '../types';
  import { styleToCss, responsiveStyleToCss, BLUR_MAP } from './styleToCss';
  import { setBlockContentContext, type BlockContentContext } from './blockContent';
  import Reveal from './Reveal.svelte';

  interface Props {
    style?: BlockStyle;
    motion?: MotionConfig;
    background?: BackgroundConfig;
    /** When true, the shell hides itself if the block reports it is empty. */
    hideIfEmpty?: boolean;
    /** Initial content state. Dynamic blocks should start as true so they can fetch and then report. */
    initialHasContent?: boolean;
    children: import('svelte').Snippet;
  }
  let { style, motion, background, hideIfEmpty = false, initialHasContent = true, children }: Props = $props();

  // svelte-ignore state_referenced_locally
  let hasContent = $state(initialHasContent ?? true);
  const blockContentContext: BlockContentContext = { set: (v) => { hasContent = v; } };
  setBlockContentContext(blockContentContext);

  const resolved = $derived(styleToCss(style));
  const inlineStyle = $derived(
    Object.entries(resolved.inline)
      .map(([k, v]) => `${k}: ${v}`)
      .join('; ')
  );

  // Background rendering
  const hasBg = $derived(!!background && background.type !== 'none');
  const bgOverlay = $derived(background?.image?.overlay);

  function bgFilterAndOpacity(bg: BackgroundConfig | undefined): string {
    if (!bg) return '';
    const parts: string[] = [];
    if (bg.blur && bg.blur !== 'none') {
      parts.push(`filter: blur(${BLUR_MAP[bg.blur] ?? '8px'})`);
    }
    if (bg.opacity !== undefined) {
      parts.push(`opacity: ${bg.opacity / 100}`);
    }
    return parts.join('; ');
  }

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

  // Only add a default `relative` class when no explicit position class is set
  // so that the Layout > Position control can override it.
  const hasPositionClass = $derived(/\b(sticky|absolute|fixed|relative)\b/.test(resolved.classes));
  const rootClasses = $derived(`${hasPositionClass ? '' : 'relative'} ${resolved.classes}`.trim());
</script>

<svelte:head>
  {@html scopedCss ? '<style>' + scopedCss + '<\/style>' : ''}
  {@html responsiveCss ? '<style>' + responsiveCss + '<\/style>' : ''}
</svelte:head>

{#if !hideIfEmpty || hasContent}
<Reveal {motion}>
  <div class={rootClasses} data-bs={scopeId} style={inlineStyle || undefined}>
    {#if hasBg && background?.type === 'image' && background.image}
      {@const fit = background.image.fit}
      {#if fit === 'repeat' || fit === 'fixed'}
        <div
          class="absolute inset-0"
          style="
            background-image: url({background.image.url});
            background-repeat: {fit === 'repeat' ? 'repeat' : 'no-repeat'};
            background-attachment: {fit === 'fixed' ? 'fixed' : 'scroll'};
            background-size: cover;
            background-position: center;
            {bgFilterAndOpacity(background)}
          "
        ></div>
      {:else}
        <img
          src={background.image.url}
          alt={background.image.alt}
          class="absolute inset-0 h-full w-full {fit === 'contain' ? 'object-contain' : 'object-cover'}"
          style={bgFilterAndOpacity(background)}
          loading="lazy"
        />
      {/if}
      {#if bgOverlay}
        <div class="absolute inset-0" style="background-color: {bgOverlay.color}; opacity: {bgOverlay.opacity / 100}"></div>
      {/if}
    {:else if hasBg && background?.type === 'video' && background.video}
      <video
        class="absolute inset-0 h-full w-full object-cover"
        src={background.video.url}
        poster={background.video.poster}
        loop={background.video.loop}
        muted={background.video.muted}
        autoplay={background.video.autoplay}
        playsInline
        style={bgFilterAndOpacity(background)}
      ></video>
    {:else if hasBg && background?.type === 'gradient' && background.gradient}
      <div
        class="absolute inset-0"
        style="background: linear-gradient({background.gradient.angle === 'radial' ? 'radial' : `${background.gradient.angle}deg`}, {background.gradient.from}, {background.gradient.to}); {bgFilterAndOpacity(background)}"
      ></div>
    {:else if hasBg && background?.type === 'color' && background.color}
      <div class="absolute inset-0" style="background-color: {background.color}; {bgFilterAndOpacity(background)}"></div>
    {/if}
    <div class="relative" style={hasBg ? 'z-index: 10;' : undefined}>
      {@render children()}
    </div>
  </div>
</Reveal>
{/if}

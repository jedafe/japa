<script lang="ts">
  import type { ContainerProps, Block } from '../types';
  import type { FetchContext } from '../data/types';
  import { blockRegistry } from '../render/blockRegistry';
  import { isBlockVisible, isBlockEmpty } from '../render/blockVisibility';
  import BlockShell from '../shell/BlockShell.svelte';
  import { getBlockContentContext } from '../shell/blockContent';

  interface Props {
    props: ContainerProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }
  let { props, fetchCtx, geoCountry, id }: Props = $props();

  // Report whether this container has any visible, non-empty children to the
  // parent BlockShell so `hide_if_empty` works for container blocks.
  const parentContentContext = getBlockContentContext();
  const hasVisibleChildren = $derived(props.children.some((c) => isBlockVisible(c, geoCountry) && !isBlockEmpty(c)));
  $effect(() => {
    parentContentContext?.set(hasVisibleChildren);
  });

  // Gap preset → CSS value (matches the spacing scale used elsewhere).
  const gapMap: Record<ContainerProps['gap'], string> = {
    none: '0',
    xs: '0.25rem',
    sm: '0.5rem',
    md: '1rem',
    lg: '1.5rem',
    xl: '2rem',
    '2xl': '3rem',
  };

  const maxWidthMap: Record<NonNullable<ContainerProps['max_width']>, string> = {
    sm: 'max-w-2xl',
    md: 'max-w-4xl',
    lg: 'max-w-5xl',
    xl: 'max-w-7xl',
    full: 'max-w-none',
  };

  const alignMap: Record<NonNullable<ContainerProps['align_items']>, string> = {
    start: 'items-start',
    center: 'items-center',
    end: 'items-end',
    stretch: 'items-stretch',
  };

  const justifyMap: Record<NonNullable<ContainerProps['justify_content']>, string> = {
    start: 'justify-start',
    center: 'justify-center',
    end: 'justify-end',
    between: 'justify-between',
    around: 'justify-around',
    evenly: 'justify-evenly',
  };

  const widthClass = $derived(maxWidthMap[props.max_width ?? 'full']);
  const minHeightStyle = $derived(
    props.min_height && props.min_height !== 'auto' ? `min-height: ${props.min_height}px` : undefined
  );

  // Resolve the effective span for a child — falls back to even distribution.
  function childSpan(block: Block, totalCols: number): number {
    if (block.span && block.span > 0 && block.span <= 12) return block.span;
    return Math.max(1, Math.floor(12 / totalCols));
  }

  // Unique ID for this container instance — used to scope responsive CSS.
  // Use the stable block id when available so SSR and client match (no hydration mismatch).
  const containerId = $derived(id ? `ctr-${id}` : `ctr-${crypto.randomUUID()}`);

  // Generate responsive CSS for per-device column overrides + child visibility.
  // Uses @container queries so they respond to the preview container width
  // (not the browser viewport), making them work in the editor preview too.
  // The desktop spans are set via inline --span CSS variable on each cell, and
  // the static <style> block uses var(--span) for the grid-column. The @container
  // rules override grid-column with !important to beat the inline --span-based value.
  const gridCss = $derived.by(() => {
    const rc = props.responsive_columns;
    const scope = `[data-ctr="${containerId}"]`;
    const parts: string[] = [];

    // Desktop: hide children marked desktop=false (min-width: 1025px)
    {
      const rules: string[] = [];
      let childIdx = 0;
      for (const child of props.children) {
        if (!isBlockVisible(child, geoCountry)) { childIdx++; continue; }
        if (child.responsive_visibility?.desktop === false) {
          rules.push(`${scope} > [data-cell="${childIdx}"] { display: none !important; }`);
        }
        childIdx++;
      }
      if (rules.length > 0) {
        parts.push(`@container (min-width: 1025px) {\n  ${rules.join('\n  ')}\n}`);
      }
    }

    for (const [device, maxWidth] of [['tablet', 1024], ['mobile', 768]] as const) {
      const rules: string[] = [];

      // Responsive column count — recompute child spans with !important to
      // override the inline --span / static style grid-column.
      const deviceCols = rc?.[device];
      if (deviceCols !== undefined && deviceCols > 0) {
        let childIdx = 0;
        for (const child of props.children) {
          if (!isBlockVisible(child, geoCountry)) { childIdx++; continue; }
          const span = childSpan(child, deviceCols);
          rules.push(`${scope} > [data-cell="${childIdx}"] { grid-column: span ${span} / span ${span} !important; }`);
          childIdx++;
        }
      }

      // Per-child responsive visibility.
      let childIdx2 = 0;
      for (const child of props.children) {
        if (!isBlockVisible(child, geoCountry)) { childIdx2++; continue; }
        const deviceVisible = child.responsive_visibility?.[device];
        if (deviceVisible === false) {
          rules.push(`${scope} > [data-cell="${childIdx2}"] { display: none !important; }`);
        }
        childIdx2++;
      }

      if (rules.length > 0) {
        parts.push(`@container (max-width: ${maxWidth}px) {\n  ${rules.join('\n  ')}\n}`);
      }
    }
    return parts.length > 0 ? parts.join('\n') : null;
  });

  // Non-grid layouts: only need visibility CSS (no column spans)
  const nonGridCss = $derived.by(() => {
    const scope = `[data-ctr="${containerId}"]`;
    const parts: string[] = [];

    // Desktop: hide children marked desktop=false
    {
      const rules: string[] = [];
      let childIdx = 0;
      for (const child of props.children) {
        if (!isBlockVisible(child, geoCountry)) { childIdx++; continue; }
        if (child.responsive_visibility?.desktop === false) {
          rules.push(`${scope} > [data-cell="${childIdx}"] { display: none !important; }`);
        }
        childIdx++;
      }
      if (rules.length > 0) {
        parts.push(`@container (min-width: 1025px) {\n  ${rules.join('\n  ')}\n}`);
      }
    }

    for (const [device, maxWidth] of [['tablet', 1024], ['mobile', 768]] as const) {
      const rules: string[] = [];
      let childIdx = 0;
      for (const child of props.children) {
        if (!isBlockVisible(child, geoCountry)) { childIdx++; continue; }
        const deviceVisible = child.responsive_visibility?.[device];
        if (deviceVisible === false) {
          rules.push(`${scope} > [data-cell="${childIdx}"] { display: none !important; }`);
        }
        childIdx++;
      }
      if (rules.length > 0) {
        parts.push(`@container (max-width: ${maxWidth}px) {\n  ${rules.join('\n  ')}\n}`);
      }
    }
    return parts.length > 0 ? parts.join('\n') : null;
  });

  // Combined CSS for <svelte:head> injection. Includes the static container-cell
  // base rules (scoped to this instance) plus the dynamic responsive overrides.
  // Merging static + dynamic into one string avoids having a component-level
  // <style> block, which conflicts with {@html} template literals in Svelte.
  const allCss = $derived.by(() => {
    const scope = `[data-ctr="${containerId}"]`;
    const parts: string[] = [];
    // Static base: mobile-first 12-col span, then desktop span via var(--span)
    parts.push(`${scope} .container-cell { grid-column: span 12 / span 12; }`);
    parts.push(`@container (min-width: 768px) { ${scope} .container-cell { grid-column: span var(--span) / span var(--span); } }`);
    // Dynamic responsive overrides
    if (gridCss) parts.push(gridCss);
    if (nonGridCss) parts.push(nonGridCss);
    return parts.join('\n');
  });
</script>

<svelte:head>
  {@html allCss ? '<style>' + allCss + '<\/style>' : ''}
</svelte:head>

<section class="w-full {widthClass} mx-auto" style={minHeightStyle}>
  {#if props.layout === 'grid'}
    <div class="container-grid grid grid-cols-12" data-ctr={containerId} style={`gap: ${gapMap[props.gap ?? 'md']}`}>
      {#each props.children as block, i (block.id)}
        {#if isBlockVisible(block, geoCountry)}
          {@const meta = blockRegistry[block.type]}
          {#if meta}
            {@const span = childSpan(block, props.columns)}
            <div class="container-cell" data-cell={i} style={`--span: ${span}`}>
              <BlockShell style={block.style} motion={block.motion} background={block.style?.background} hideIfEmpty={block.visibility_rules?.hide_if_empty} initialHasContent={!isBlockEmpty(block)}>
                {@const Component = meta.component}
                {#if block.type === 'custom'}
                  <Component props={block.props} blockDefId={block.block_def_id} {fetchCtx} {geoCountry} id={block.id} />
                {:else}
                  <Component props={block.props} {fetchCtx} {geoCountry} id={block.id} />
                {/if}
              </BlockShell>
            </div>
          {/if}
        {/if}
      {/each}
    </div>
  {:else if props.layout === 'flex'}
    <div
      class="flex {props.wrap ? 'flex-wrap' : 'flex-nowrap'} {alignMap[props.align_items ?? 'start']} {justifyMap[props.justify_content ?? 'start']}"
      data-ctr={containerId}
      style={`gap: ${gapMap[props.gap ?? 'md']}`}
    >
      {#each props.children as block, i (block.id)}
        {#if isBlockVisible(block, geoCountry)}
          {@const meta = blockRegistry[block.type]}
          {#if meta}
            <div class="flex-1 min-w-0" data-cell={i}>
              <BlockShell style={block.style} motion={block.motion} background={block.style?.background} hideIfEmpty={block.visibility_rules?.hide_if_empty} initialHasContent={!isBlockEmpty(block)}>
                {@const Component = meta.component}
                {#if block.type === 'custom'}
                  <Component props={block.props} blockDefId={block.block_def_id} {fetchCtx} {geoCountry} id={block.id} />
                {:else}
                  <Component props={block.props} {fetchCtx} {geoCountry} id={block.id} />
                {/if}
              </BlockShell>
            </div>
          {/if}
        {/if}
      {/each}
    </div>
  {:else}
    <!-- stack: vertical, like the flat section model but nestable -->
    <div class="flex flex-col" data-ctr={containerId} style={`gap: ${gapMap[props.gap ?? 'md']}`}>
      {#each props.children as block, i (block.id)}
        {#if isBlockVisible(block, geoCountry)}
          {@const meta = blockRegistry[block.type]}
          {#if meta}
            <div data-cell={i}>
              <BlockShell style={block.style} motion={block.motion} background={block.style?.background} hideIfEmpty={block.visibility_rules?.hide_if_empty} initialHasContent={!isBlockEmpty(block)}>
                {@const Component = meta.component}
                {#if block.type === 'custom'}
                  <Component props={block.props} blockDefId={block.block_def_id} {fetchCtx} {geoCountry} id={block.id} />
                {:else}
                  <Component props={block.props} {fetchCtx} {geoCountry} id={block.id} />
                {/if}
              </BlockShell>
            </div>
          {/if}
        {/if}
      {/each}
    </div>
  {/if}
</section>

<script lang="ts">
  import type { GuestHeroContainerProps, Block, BlockStyle } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { blockRegistry } from '../../render/blockRegistry';
  import BlockShell from '../../shell/BlockShell.svelte';
  import { isBlockVisible, isBlockEmpty } from '../../render/blockVisibility';

  interface Props {
    props: GuestHeroContainerProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, fetchCtx, geoCountry, id = 'guest-hero' }: Props = $props();

  const leftTypes = ['guest_hero_text', 'guest_hero_stats'];
  const rightTypes = ['guest_hero_image', 'guest_hero_destinations', 'guest_hero_community'];

  function defaultStyle(type: string): BlockStyle | undefined {
    if (type === 'guest_hero_destinations') {
      return { layout: { position: 'absolute', top: 0, left: 0, z_index: 10 } };
    }
    if (type === 'guest_hero_community') {
      return { layout: { position: 'absolute', bottom: 0, right: 0, z_index: 10 } };
    }
    return undefined;
  }

  const defaultChildren: Block[] = $derived([
    { id: `${id}-text`, type: 'guest_hero_text', visible: true, props: {} },
    { id: `${id}-image`, type: 'guest_hero_image', visible: true, props: {} },
    { id: `${id}-stats`, type: 'guest_hero_stats', visible: true, props: {} },
    { id: `${id}-destinations`, type: 'guest_hero_destinations', visible: true, props: {}, style: defaultStyle('guest_hero_destinations') },
    { id: `${id}-community`, type: 'guest_hero_community', visible: true, props: {}, style: defaultStyle('guest_hero_community') },
  ]);

  const children = $derived(props.children && props.children.length > 0 ? props.children : defaultChildren);

  const leftChildren = $derived(children.filter((b) => leftTypes.includes(b.type)));
  const rightChildren = $derived(children.filter((b) => rightTypes.includes(b.type)));

  function mergedStyle(block: Block): BlockStyle | undefined {
    const ds = defaultStyle(block.type);
    if (!block.style && !ds) return undefined;
    return { ...ds, ...block.style };
  }

  function renderableComponent(block: Block) {
    const meta = blockRegistry[block.type];
    if (!meta || !isBlockVisible(block, geoCountry)) return null;
    return { Component: meta.component, style: mergedStyle(block) };
  }
</script>

<!-- Ref: japaconnect-engineering/sections/16-page-builder.md -->
<section class="guest-hero" data-hero={id} aria-label="Hero">
  <div class="guest-hero-grid">
    <div class="guest-hero-left">
      {#each leftChildren as block (block.id)}
        {@const r = renderableComponent(block)}
        {#if r}
          {@const Component = r.Component}
          <BlockShell style={r.style} motion={block.motion} background={block.style?.background} hideIfEmpty={block.visibility_rules?.hide_if_empty} initialHasContent={!isBlockEmpty(block)}>
            <Component props={block.props} {fetchCtx} {geoCountry} id={block.id} />
          </BlockShell>
        {/if}
      {/each}
    </div>
    <div class="guest-hero-right">
      {#each rightChildren as block (block.id)}
        {@const r = renderableComponent(block)}
        {#if r}
          {@const Component = r.Component}
          <BlockShell style={r.style} motion={block.motion} background={block.style?.background} hideIfEmpty={block.visibility_rules?.hide_if_empty} initialHasContent={!isBlockEmpty(block)}>
            <Component props={block.props} {fetchCtx} {geoCountry} id={block.id} />
          </BlockShell>
        {/if}
      {/each}
    </div>
  </div>
</section>

<style>
  .guest-hero-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 2rem;
    align-items: center;
  }

  .guest-hero-left,
  .guest-hero-right {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .guest-hero-right {
    position: relative;
  }

  @media (max-width: 1024px) {
    .guest-hero-grid {
      grid-template-columns: 1fr;
    }
  }
</style>

<script lang="ts">
  import type { GuestHeroContainerProps, Block } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { blockRegistry } from '../../render/blockRegistry';
  import BlockShell from '../../shell/BlockShell.svelte';
  import { isBlockVisible, isBlockEmpty } from '../../render/blockVisibility';
  import { responsiveVisibilityCss } from '../../shell/styleToCss';
  import ScopedCss from '../../shell/ScopedCss.svelte';

  interface Props {
    props: GuestHeroContainerProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, fetchCtx, geoCountry, id = 'guest-hero' }: Props = $props();

  // `guest_hero_image` anchors the right column; every other child type
  // (hero text/stats plus any other block the admin nests here) renders
  // in the left column so no child is silently dropped.
  const rightTypes = ['guest_hero_image'];

  const defaultChildren: Block[] = $derived([
    { id: `${id}-text`, type: 'guest_hero_text', visible: true, props: {} as any },
    {
      id: `${id}-image`,
      type: 'guest_hero_image',
      visible: true,
      props: {
        children: [
          { id: `${id}-destinations`, type: 'guest_hero_destinations', visible: true, props: {} as any },
          { id: `${id}-community`, type: 'guest_hero_community', visible: true, props: {} as any },
        ]
      } as any
    },
    { id: `${id}-stats`, type: 'guest_hero_stats', visible: true, props: {} as any },
  ] as Block[]);

  const children = $derived(props.children && props.children.length > 0 ? props.children : defaultChildren);

  const leftChildren = $derived(children.filter((b) => !rightTypes.includes(b.type)));
  const rightChildren = $derived(children.filter((b) => rightTypes.includes(b.type)));

  function renderableComponent(block: Block) {
    const meta = blockRegistry[block.type];
    if (!meta || !isBlockVisible(block, geoCountry)) return null;
    return { Component: meta.component, style: block.style };
  }

  // Per-device visibility rules for hero children (scopeKey = block.id).
  const visibilityCss = $derived(responsiveVisibilityCss(children));
</script>

<ScopedCss css={visibilityCss} />

<!-- Ref: japaconnect-engineering/sections/16-page-builder.md -->
<section class="guest-hero" data-hero={id} aria-label="Hero">
  <div class="guest-hero-grid">
    <div class="guest-hero-left">
      {#each leftChildren as block (block.id)}
        {@const r = renderableComponent(block)}
        {#if r}
          {@const Component = r.Component}
          <BlockShell style={r.style} motion={block.motion} background={block.style?.background} hideIfEmpty={block.visibility_rules?.hide_if_empty} initialHasContent={!isBlockEmpty(block)} scopeKey={block.id}>
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
          <BlockShell style={r.style} motion={block.motion} background={block.style?.background} hideIfEmpty={block.visibility_rules?.hide_if_empty} initialHasContent={!isBlockEmpty(block)} scopeKey={block.id}>
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

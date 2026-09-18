<script lang="ts">
  // Ref: japaconnect-engineering/sections/16-page-builder.md
  // Shared renderer for guest block children — iterates nested blocks and
  // renders each through the block registry with visibility rules and
  // BlockShell chrome (style/motion/hide-if-empty). `tag`/`class` optionally
  // wrap each child in a container element (e.g. <li class="destination-item">);
  // omit `tag` to render the BlockShell directly.
  import type { Block } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { blockRegistry } from '../../render/blockRegistry';
  import { isBlockVisible, isBlockEmpty } from '../../render/blockVisibility';
  import BlockShell from '../../shell/BlockShell.svelte';
  import ScopedCss from '../../shell/ScopedCss.svelte';
  import { responsiveVisibilityCss } from '../../shell/styleToCss';

  interface Props {
    blocks: Block[];
    fetchCtx?: FetchContext;
    geoCountry?: string;
    /** Wrapper element for each child (e.g. 'li', 'div'). Omit for no wrapper. */
    tag?: string;
    /** Class applied to the wrapper element. */
    class?: string;
  }

  let { blocks, fetchCtx, geoCountry, tag, class: cls }: Props = $props();

  // Per-device visibility rules for these children — scoped to each child's
  // BlockShell `data-bs` attribute (scopeKey = block.id).
  const visibilityCss = $derived(responsiveVisibilityCss(blocks));
</script>

<ScopedCss css={visibilityCss} />

{#snippet childShell(block: Block)}
  <BlockShell
    style={block.style}
    motion={block.motion}
    background={block.style?.background}
    hideIfEmpty={block.visibility_rules?.hide_if_empty}
    initialHasContent={!isBlockEmpty(block)}
    scopeKey={block.id}
  >
    {@const Component = blockRegistry[block.type].component}
    {#if block.type === 'custom'}
      <Component props={block.props} blockDefId={block.block_def_id} {fetchCtx} {geoCountry} id={block.id} />
    {:else}
      <Component props={block.props} {fetchCtx} {geoCountry} id={block.id} />
    {/if}
  </BlockShell>
{/snippet}

{#each blocks as block (block.id)}
  {#if isBlockVisible(block, geoCountry) && blockRegistry[block.type]}
    {#if tag}
      <svelte:element this={tag} class={cls} data-item={block.id}>
        {@render childShell(block)}
      </svelte:element>
    {:else}
      {@render childShell(block)}
    {/if}
  {/if}
{/each}

<script lang="ts">
  import type { Block } from '../types';
  import type { FetchContext } from '../data/types';
  import { blockRegistry } from './blockRegistry';
  import BlockShell from '../shell/BlockShell.svelte';

  interface Props {
    blocks: Block[];
    fetchCtx?: FetchContext;
    geoCountry?: string;
  }
  let { blocks, fetchCtx, geoCountry }: Props = $props();
</script>

<div class="landing-page min-h-screen bg-[var(--color-bg)]">
  {#each blocks as block (block.id)}
    {#if block.visible}
      {@const meta = blockRegistry[block.type]}
      {#if meta}
        <BlockShell style={block.style} motion={block.motion} background={block.style?.background}>
          {@const Component = meta.component}
          <Component props={block.props} {fetchCtx} {geoCountry} />
        </BlockShell>
      {/if}
    {/if}
  {/each}
</div>

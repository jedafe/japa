<script lang="ts">
  import type { Block, GuestHomeData } from '../types';
  import type { FetchContext } from '../data/types';
  import { blockRegistry } from './blockRegistry';
  import BlockShell from '../shell/BlockShell.svelte';
  import { isBlockVisible, isBlockEmpty } from './blockVisibility';
  import { setGuestHomeData } from '../data/guestHomeContext';
  import GuestBlockStyles from '../blocks/guest/GuestBlockStyles.svelte';

  interface Props {
    blocks: Block[];
    fetchCtx?: FetchContext;
    geoCountry?: string;
    guestHome?: GuestHomeData;
  }
  let { blocks, fetchCtx, geoCountry, guestHome }: Props = $props();

  // svelte-ignore state_referenced_locally
  const guestHomeStore = setGuestHomeData(guestHome);
  $effect(() => {
    guestHomeStore.set(guestHome);
  });

  // Load the shared guest-block stylesheet only when at least one guest_*
  // block is on the page, so the split guest blocks render with the same
  // visual styling as the legacy monolithic block.
  const hasGuestBlocks = $derived(blocks.some((b) => b.type.startsWith('guest_')));

  // Generate per-device visibility CSS for top-level blocks.
  // Uses @container queries so they respond to the preview container width
  // (not the browser viewport), making them work in the editor preview too.
  // Container children handle their own visibility in ContainerBlock.svelte.
  const responsiveVisibilityCss = $derived.by(() => {
    const parts: string[] = [];
    for (const block of blocks) {
      if (!isBlockVisible(block, geoCountry) || !block.responsive_visibility) continue;
      const scope = `[data-rv="${block.id}"]`;
      // Desktop: hide when container is wider than 1024px
      if (block.responsive_visibility.desktop === false) {
        parts.push(`@container (min-width: 1025px) { ${scope} { display: none; } }`);
      }
      // Tablet: hide when container is ≤ 1024px
      if (block.responsive_visibility.tablet === false) {
        parts.push(`@container (max-width: 1024px) { ${scope} { display: none; } }`);
      }
      // Mobile: hide when container is ≤ 768px
      if (block.responsive_visibility.mobile === false) {
        parts.push(`@container (max-width: 768px) { ${scope} { display: none; } }`);
      }
    }
    return parts.length > 0 ? parts.join('\n') : null;
  });
</script>

<svelte:head>
  {@html responsiveVisibilityCss ? '<style>' + responsiveVisibilityCss + '<\/style>' : ''}
</svelte:head>

{#if hasGuestBlocks}
  <GuestBlockStyles />
{/if}

<div class="landing-page min-h-screen bg-[var(--color-bg)] {hasGuestBlocks ? 'guest-light' : ''}" style="container-type: inline-size">
  {#each blocks as block (block.id)}
    {#if isBlockVisible(block, geoCountry)}
      {@const meta = blockRegistry[block.type]}
      {#if meta}
        <div data-rv={block.id}>
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

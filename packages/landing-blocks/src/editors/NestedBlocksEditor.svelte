<script lang="ts">
  import type { Block, GeoContext } from '../types';
  import type { Component } from 'svelte';
  import { blockRegistry } from '../render/blockRegistry';
  import { Dialog } from 'bits-ui';
  import * as LucideIcons from 'lucide-svelte';

  interface Props {
    blocks: Block[];
    onchange: (v: Block[]) => void;
    allowedTypes: Block['type'][];
    mediaPickerComponent?: Component;
    geoCtx?: GeoContext;
  }
  let { blocks, onchange, allowedTypes, mediaPickerComponent, geoCtx }: Props = $props();

  let showAddDialog = $state(false);
  let expandedId = $state<string | null>(null);

  function getIcon(name: string): Component | null {
    const found = (LucideIcons as Record<string, unknown>)[name];
    return (found as Component) ?? null;
  }

  function addBlock(type: Block['type']) {
    const newBlock: Block = {
      id: crypto.randomUUID(),
      type,
      visible: true,
      props: {} as Block['props'],
    } as Block;
    onchange([...blocks, newBlock]);
    expandedId = newBlock.id;
    showAddDialog = false;
  }

  function removeBlock(id: string) {
    onchange(blocks.filter((b) => b.id !== id));
    if (expandedId === id) expandedId = null;
  }

  function moveUp(idx: number) {
    if (idx === 0) return;
    const reordered = [...blocks];
    [reordered[idx - 1], reordered[idx]] = [reordered[idx], reordered[idx - 1]];
    onchange(reordered);
  }

  function moveDown(idx: number) {
    if (idx === blocks.length - 1) return;
    const reordered = [...blocks];
    [reordered[idx], reordered[idx + 1]] = [reordered[idx + 1], reordered[idx]];
    onchange(reordered);
  }

  function toggleExpand(id: string) {
    expandedId = expandedId === id ? null : id;
  }
</script>

<div class="space-y-2">
  {#each blocks as block, i (block.id)}
    {@const meta = blockRegistry[block.type]}
    <div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)]">
      <div class="flex items-center gap-2 p-2">
        {#if meta}
          {@const Icon = getIcon(meta.icon)}
          {#if Icon}
            <Icon size={14} />
          {/if}
        {/if}
        <button type="button" onclick={() => toggleExpand(block.id)} class="flex-1 text-left text-sm text-[var(--color-text)]">
          {meta?.label ?? block.type}
        </button>
        <button type="button" onclick={() => moveUp(i)} disabled={i === 0} class="text-xs text-[var(--color-muted)] disabled:opacity-30">&#8593;</button>
        <button type="button" onclick={() => moveDown(i)} disabled={i === blocks.length - 1} class="text-xs text-[var(--color-muted)] disabled:opacity-30">&#8595;</button>
        <button type="button" onclick={() => removeBlock(block.id)} class="text-xs text-[var(--color-danger)]">&#10005;</button>
      </div>
    </div>
  {/each}

  <button
    type="button"
    onclick={() => (showAddDialog = true)}
    class="w-full rounded-[var(--radius)] border border-dashed border-[var(--color-border)] py-2 text-sm text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
  >
    + Add block
  </button>

  <Dialog.Root bind:open={showAddDialog}>
    <Dialog.Portal>
      <Dialog.Overlay class="fixed inset-0 bg-black/50 z-50" />
      <Dialog.Content class="fixed left-1/2 top-1/2 z-50 max-h-[80vh] w-full max-w-lg -translate-x-1/2 -translate-y-1/2 overflow-y-auto rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 shadow-xl">
        <Dialog.Title class="mb-4 text-lg font-semibold text-[var(--color-text)]">Add Block</Dialog.Title>
        <div class="grid grid-cols-2 gap-2">
          {#each allowedTypes as type}
            {@const meta = blockRegistry[type]}
            {#if meta}
              {@const Icon = getIcon(meta.icon)}
              <button
                type="button"
                onclick={() => addBlock(type)}
                class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] p-3 text-left hover:bg-[var(--color-surface-subtle)]"
              >
                {#if Icon}
                  <Icon size={16} />
                {/if}
                <div>
                  <p class="text-sm font-medium text-[var(--color-text)]">{meta.label}</p>
                  <p class="text-xs text-[var(--color-muted)]">{meta.category}</p>
                </div>
              </button>
            {/if}
          {/each}
        </div>
        <Dialog.Close class="absolute right-4 top-4 text-[var(--color-muted)] hover:text-[var(--color-text)]" aria-label="Close">&#10005;</Dialog.Close>
      </Dialog.Content>
    </Dialog.Portal>
  </Dialog.Root>
</div>

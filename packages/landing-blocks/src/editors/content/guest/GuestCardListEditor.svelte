<script lang="ts">
  import { DragDropProvider, DragOverlay } from '@dnd-kit/svelte';
  import { isSortable } from '@dnd-kit/svelte/sortable';
  import type { Snippet } from 'svelte';
  import { Plus } from 'lucide-svelte';
  import SortableCard from './SortableCard.svelte';

  interface Props {
    items: any[];
    getId?: (item: any) => string;
    group?: string;
    card: Snippet<[any, (v: any) => void, () => void]>;
    onchange: (items: any[]) => void;
    add: () => any;
    id?: string;
  }

  let { items, getId = (i) => i.id, group, card, onchange, add, id = 'card-list' }: Props = $props();

  const groupId = $derived(group ?? `card-list-${id}`);

  function handleDragEnd(event: any) {
    if (event?.canceled) return;
    const { source, target } = event.operation ?? {};
    if (!source || !target || !isSortable(target)) return;
    const sourceId = source.id as string;
    const targetId = target.id as string;
    if (sourceId === targetId) return;
    const from = items.findIndex((i) => getId(i) === sourceId);
    const to = items.findIndex((i) => getId(i) === targetId);
    if (from < 0 || to < 0) return;
    const next = [...items];
    const [moved] = next.splice(from, 1);
    next.splice(to, 0, moved);
    onchange(next);
  }

  function update(item: any, value: any) {
    const id = getId(item);
    const i = items.findIndex((x) => getId(x) === id);
    if (i >= 0) {
      const next = [...items];
      next[i] = value;
      onchange(next);
    }
  }

  function remove(item: any) {
    const id = getId(item);
    onchange(items.filter((x) => getId(x) !== id));
  }
</script>

<DragDropProvider onDragEnd={handleDragEnd}>
  <div class="space-y-2">
    {#each items as item, index (getId(item))}
      <SortableCard id={getId(item)} {index} group={groupId} onRemove={() => remove(item)}>
        {@render card(item, (v) => update(item, v), () => remove(item))}
      </SortableCard>
    {/each}
    <button
      type="button"
      onclick={add}
      class="flex w-full items-center justify-center gap-1 rounded border border-dashed border-[var(--color-border)] px-2 py-1.5 text-sm text-[var(--color-muted)] hover:bg-[var(--color-surface)]"
    >
      <Plus size={16} />
      Add card
    </button>
  </div>
  <DragOverlay>
    <div class="rounded border border-[var(--color-primary)] bg-[var(--color-background)] px-3 py-2 text-sm shadow-lg">
      Card
    </div>
  </DragOverlay>
</DragDropProvider>

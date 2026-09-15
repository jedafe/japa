<script lang="ts">
  import { createSortable } from '@dnd-kit/svelte/sortable';
  import { GripVertical, Trash2 } from 'lucide-svelte';
  import type { Snippet } from 'svelte';

  interface Props {
    id: string;
    index: number;
    group: string;
    children: Snippet;
    onRemove?: () => void;
  }

  let { id, index, group, children, onRemove }: Props = $props();

  const sortableInput = $state({
    get id() {
      return id;
    },
    get index() {
      return index;
    },
    get group() {
      return group;
    },
    get data() {
      return { source: 'card-list' as const, id, index };
    },
  });
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const sortable = createSortable(sortableInput as any);
</script>

<div
  {@attach sortable.attach}
  class="group flex items-start gap-2 rounded border border-[var(--color-border)] bg-[var(--color-surface)] p-2 transition-opacity {sortable.isDragging ? 'opacity-40 ring-2 ring-[var(--color-primary)]' : ''}"
>
  <div
    {@attach sortable.attachHandle}
    class="cursor-grab touch-none p-1 text-[var(--color-muted)]"
    aria-hidden="true"
  >
    <GripVertical size={16} />
  </div>
  <div class="flex-1 min-w-0">
    {@render children()}
  </div>
  {#if onRemove}
    <button
      type="button"
      class="p-1 text-[var(--color-muted)] hover:text-[var(--color-danger)]"
      aria-label="Remove card"
      onclick={(e) => {
        e.stopPropagation();
        onRemove();
      }}
    >
      <Trash2 size={16} />
    </button>
  {/if}
</div>

<script lang="ts">
	// Ref: japaconnect-engineering/sections/16-page-builder.md (dnd-kit foundation)
	import { createDraggable } from '@dnd-kit/svelte';
	import type { Component } from 'svelte';
	import type { BlockMeta } from '@japaconnect/landing-blocks';

	let {
		blockType,
		meta,
		getIcon,
		onAdd,
	}: {
		blockType: string;
		meta: BlockMeta;
		getIcon: (name: string) => Component | null;
		/** Click-to-add handler — appends the block as a new top-level section. */
		onAdd?: (blockType: string) => void;
	} = $props();

	const draggableInput = $state({
		get id() {
			return `palette-${blockType}`;
		},
		get data() {
			return { source: 'palette' as const, blockType, meta };
		},
	});
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const draggable = createDraggable(draggableInput as any);

	const Icon = $derived(getIcon(meta.icon));
</script>

<div
	{@attach draggable.attach}
	class="flex cursor-grab items-center gap-2 rounded-md border px-2.5 py-2 text-sm transition-colors select-none hover:border-[var(--primary)]/50"
	class:opacity-50={draggable.isDragging}
	class:ring-2={draggable.isDragging}
	style="border-color: var(--color-border); background-color: var(--color-background)"
	role="button"
	tabindex="0"
	title="{meta.label} — drag or click to add"
	onclick={() => onAdd?.(blockType)}
	onkeydown={(e) => { if (e.key === 'Enter' && onAdd) onAdd(blockType); }}
>
	{#if Icon}
		<Icon size={14} />
	{/if}
	<span class="flex-1 truncate text-[var(--foreground)]">{meta.label}</span>
</div>

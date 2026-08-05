<script lang="ts">
	// Ref: japaconnect-engineering/sections/16-page-builder.md (dnd-kit foundation)
	import { createSortable } from '@dnd-kit/svelte/sortable';
	import { GripVertical } from 'lucide-svelte';
	import type { Component } from 'svelte';
	import type { Block, BlockMeta } from '@japaconnect/landing-blocks';

	let {
		block,
		index,
		selected,
		meta,
		getIcon,
		onSelect,
	}: {
		block: Block;
		index: number;
		selected: boolean;
		meta: BlockMeta | undefined;
		getIcon: (name: string) => Component | null;
		onSelect: () => void;
	} = $props();

	const sortableInput = $state({
		get id() {
			return block.id;
		},
		get index() {
			return index;
		},
		get data() {
			return { source: 'canvas' as const, blockType: block.type, index };
		},
	});
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const sortable = createSortable(sortableInput as any);

	const Icon = $derived(meta ? getIcon(meta.icon) : null);
	const label = $derived(block.label ?? meta?.label ?? block.type);
</script>

<div
	{@attach sortable.attach}
	class="group flex items-center gap-2 rounded px-2 py-1.5 text-sm transition-colors {selected
		? 'bg-[var(--surface)]'
		: 'hover:bg-[var(--surface)]'} {sortable.isDragging ? 'opacity-40 ring-2 ring-[var(--primary)]' : ''}"
	role="button"
	tabindex="0"
	aria-label="Select section {label}"
	onclick={onSelect}
	onkeydown={(e) => {
		if (e.key === 'Enter' || e.key === ' ') {
			e.preventDefault();
			onSelect();
		}
	}}
>
	<div
		{@attach sortable.attachHandle}
		class="cursor-grab touch-none p-0.5 text-[var(--muted)]"
		aria-hidden="true"
	>
		<GripVertical class="h-3.5 w-3.5" />
	</div>
	{#if Icon}
		<Icon size={14} />
	{/if}
	<span class="flex-1 truncate text-[var(--foreground)]">{label}</span>
	{#if !block.visible}<span class="text-xs text-[var(--muted)]">hidden</span>{/if}
	<span class="font-mono text-[10px] text-[var(--muted)]">#{index + 1}</span>
</div>

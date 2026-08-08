<script lang="ts">
	// Renders a container block's children as a nested, sortable list inside
	// the sections sidebar. Each child is a dnd-kit sortable item (group =
	// container ID) enabling drag-to-reorder within the container. Children
	// also have remove and move-to-top-level buttons. In grid mode, each
	// selected child shows a 12-column span slider for visual resizing.
	// A droppable zone at the end accepts palette drags and canvas-section
	// drags, inserting the block into this container's children.
	import { createSortable } from '@dnd-kit/svelte/sortable';
	import { createDroppable } from '@dnd-kit/svelte';
	import { GripVertical, ChevronUp, ChevronDown, X, ArrowUpRight } from 'lucide-svelte';
	import type { Component } from 'svelte';
	import { blockRegistry, type Block, type BlockMeta, type ContainerProps } from '@japaconnect/landing-blocks';

	let {
		containerId,
		children,
		layout,
		columns,
		selectedBlockId,
		getIcon,
		onSelect,
		onReorder,
		onRemove,
		onMoveOut,
		onSpanChange,
	}: {
		containerId: string;
		children: Block[];
		layout: ContainerProps['layout'];
		columns: number;
		selectedBlockId: string | null;
		getIcon: (name: string) => Component | null;
		onSelect: (id: string) => void;
		onReorder: (containerId: string, fromIdx: number, toIdx: number) => void;
		onRemove: (containerId: string, childId: string) => void;
		onMoveOut: (containerId: string, childId: string) => void;
		onSpanChange: (containerId: string, childId: string, span: number) => void;
	} = $props();

	function meta(type: string): BlockMeta | undefined {
		return (blockRegistry as Record<string, BlockMeta>)[type];
	}

	// Effective span for a child — falls back to even distribution.
	function effectiveSpan(child: Block): number {
		if (child.span && child.span > 0 && child.span <= 12) return child.span;
		return Math.max(1, Math.floor(12 / columns));
	}

	// Droppable zone for accepting palette/canvas drags into this container.
	const dropInput = $state({
		get id() {
			return `dropzone-${containerId}`;
		},
		get data() {
			return { source: 'container-dropzone' as const, containerId };
		},
	});
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const droppable = createDroppable(dropInput as any);

	// Create a sortable for each child. The `group` is the container ID so
	// dnd-kit knows these items belong together for reordering.
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const sortables = $derived.by(() => {
		return children.map((child, i) => {
			const input = $state({
				get id() {
					return child.id;
				},
				get index() {
					return i;
				},
				get group() {
					return containerId;
				},
				get data() {
					return { source: 'container-child' as const, containerId, blockType: child.type, index: i };
				},
			});
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			return createSortable(input as any);
		});
	});
</script>

<div class="ml-3 border-l pl-2" style="border-color: var(--color-border)">
	{#each children as child, i (child.id)}
		{@const m = meta(child.type)}
		{@const sortable = sortables[i]}
		<div
			{@attach sortable.attach}
			class="group flex items-center gap-1.5 rounded px-2 py-1 text-xs transition-colors {selectedBlockId === child.id
				? 'bg-[var(--surface)]'
				: 'hover:bg-[var(--surface)]'} {sortable.isDragging ? 'opacity-40 ring-2 ring-[var(--primary)]' : ''}"
			role="button"
			tabindex="0"
			aria-label="Select child section"
			onclick={onSelect.bind(null, child.id)}
			onkeydown={(e) => {
				if (e.key === 'Enter' || e.key === ' ') {
					e.preventDefault();
					onSelect(child.id);
				}
			}}
		>
			<div
				{@attach sortable.attachHandle}
				class="cursor-grab touch-none text-[var(--muted)]"
				aria-hidden="true"
			>
				<GripVertical class="h-3 w-3" />
			</div>
			{#if m}
				{@const Icon = getIcon(m.icon)}
				{#if Icon}<Icon size={12} />{/if}
			{/if}
			<span class="flex-1 truncate text-[var(--foreground)]">{child.label ?? m?.label ?? child.type}</span>
			{#if !child.visible}<span class="text-[10px] text-[var(--muted)]">hidden</span>{/if}
			{#if layout === 'grid'}<span class="font-mono text-[10px] text-[var(--muted)]">{effectiveSpan(child)}/12</span>{/if}
			<!-- Quick actions (always visible for accessibility) -->
			<button type="button" onclick={() => onReorder(containerId, i, i - 1)} disabled={i === 0} class="text-[var(--muted)] disabled:opacity-20 hover:text-[var(--foreground)]" aria-label="Move up">
				<ChevronUp class="h-3 w-3" />
			</button>
			<button type="button" onclick={() => onReorder(containerId, i, i + 1)} disabled={i === children.length - 1} class="text-[var(--muted)] disabled:opacity-20 hover:text-[var(--foreground)]" aria-label="Move down">
				<ChevronDown class="h-3 w-3" />
			</button>
			<button type="button" onclick={() => onMoveOut(containerId, child.id)} class="text-[var(--muted)] hover:text-[var(--foreground)]" aria-label="Move to top level" title="Move to top level">
				<ArrowUpRight class="h-3 w-3" />
			</button>
			<button type="button" onclick={() => onRemove(containerId, child.id)} class="text-[var(--danger)] hover:opacity-80" aria-label="Remove child">
				<X class="h-3 w-3" />
			</button>
		</div>
		<!-- Span slider — visible when this child is selected and container is in grid mode -->
		{#if layout === 'grid' && selectedBlockId === child.id}
			<div class="flex items-center gap-2 px-3 py-1">
				<span class="text-[9px] text-[var(--muted)] whitespace-nowrap">span</span>
				<input
					type="range"
					min="1"
					max="12"
					value={effectiveSpan(child)}
					oninput={(e) => onSpanChange(containerId, child.id, Number(e.currentTarget.value))}
					class="flex-1 h-1 cursor-pointer"
					style="accent-color: var(--color-primary)"
					aria-label="Column span (1-12)"
				/>
				<!-- Visual 12-col bar -->
				<div class="flex gap-px" style="width: 48px">
					{#each Array(12) as _, j}
						<div
							class="h-2 flex-1 rounded-sm"
							style={j < effectiveSpan(child) ? 'background: var(--color-primary)' : 'background: var(--color-border)'}
						></div>
					{/each}
				</div>
			</div>
		{/if}
	{/each}

	<!-- Drop zone — accepts palette and canvas drags -->
	<div
		{@attach droppable.attach}
		class="mt-1 rounded border border-dashed px-2 py-1.5 text-center text-[10px] transition-colors {droppable.isDropTarget ? 'border-[var(--primary)] bg-[var(--surface)] text-[var(--primary)]' : 'text-[var(--muted)]'}"
		style="border-color: var(--color-border)"
	>
		{droppable.isDropTarget ? 'Drop to add to container' : '+ Drop blocks here'}
	</div>
</div>

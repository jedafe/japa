<script lang="ts">
	// Renders a container block's children as a nested, sortable tree inside
	// the sections sidebar. Each child is a dnd-kit sortable item (group =
	// container ID + slot key) enabling drag-to-reorder within the slot.
	// Children also have remove and move-to-top-level buttons. In grid mode,
	// each selected child shows a 12-column span slider for visual resizing.
	// Container-like children can be expanded to show their own children,
	// so nested layouts (e.g. guest_home → guest_hero → guest_hero_image) are
	// editable and reorderable at any depth. Accordion/tabs render a labeled
	// group per item/tab slot.
	import { createSortable } from '@dnd-kit/svelte/sortable';
	import { createDroppable } from '@dnd-kit/svelte';
	import { GripVertical, ChevronUp, ChevronDown, X, ArrowUpRight } from 'lucide-svelte';
	import type { Component } from 'svelte';
	import { blockRegistry, type Block, type BlockMeta } from '@japaconnect/landing-blocks';
	import { isContainerLike, childrenOf, childSlotsOf, getContainerLayout, getContainerColumns, DEFAULT_SLOT } from '$lib/utils/blocks';
	import ContainerChildren from './ContainerChildren.svelte';

	let {
		containerId,
		slotKey = DEFAULT_SLOT,
		children,
		layout,
		columns,
		selectedBlockId,
		getIcon,
		expandedContainers,
		toggleExpand,
		onSelect,
		onReorder,
		onRemove,
		onMoveOut,
		onSpanChange,
	}: {
		containerId: string;
		/** Which child slot this list renders ('children', 'slides', 'item:N', 'tab:N'). */
		slotKey?: string;
		children: Block[];
		layout: 'stack' | 'grid' | 'flex';
		columns: number;
		selectedBlockId: string | null;
		getIcon: (name: string) => Component | null;
		expandedContainers: Set<string>;
		toggleExpand: (id: string) => void;
		onSelect: (id: string) => void;
		onReorder: (containerId: string, slotKey: string, fromIdx: number, toIdx: number) => void;
		onRemove: (containerId: string, slotKey: string, childId: string) => void;
		onMoveOut: (containerId: string, slotKey: string, childId: string) => void;
		onSpanChange: (containerId: string, slotKey: string, childId: string, span: number) => void;
	} = $props();

	function meta(type: string): BlockMeta | undefined {
		return (blockRegistry as Record<string, BlockMeta>)[type];
	}

	// Effective span for a child — falls back to even distribution.
	function effectiveSpan(child: Block): number {
		if (child.span && child.span > 0 && child.span <= 12) return child.span;
		return Math.max(1, Math.floor(12 / columns));
	}

	// Sortable/droppable identity — scoped to this container's exact child
	// slot so reorder and drop operations hit the right array.
	const groupId = $derived(`${containerId}::${slotKey}`);

	// Droppable zone for accepting palette/canvas drags into this slot.
	const dropInput = $state({
		get id() {
			return `dropzone-${containerId}-${slotKey}`;
		},
		get data() {
			return { source: 'container-dropzone' as const, containerId, slotKey };
		},
	});
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const droppable = createDroppable(dropInput as any);

	// Create a sortable for each child. The `group` scopes reordering to this
	// container slot.
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
					return groupId;
				},
				get data() {
					return { source: 'container-child' as const, containerId, slotKey, blockType: child.type, index: i };
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
			<button type="button" onclick={() => onReorder(containerId, slotKey, i, i - 1)} disabled={i === 0} class="text-[var(--muted)] disabled:opacity-20 hover:text-[var(--foreground)]" aria-label="Move up">
				<ChevronUp class="h-3 w-3" />
			</button>
			<button type="button" onclick={() => onReorder(containerId, slotKey, i, i + 1)} disabled={i === children.length - 1} class="text-[var(--muted)] disabled:opacity-20 hover:text-[var(--foreground)]" aria-label="Move down">
				<ChevronDown class="h-3 w-3" />
			</button>
			<button type="button" onclick={() => onMoveOut(containerId, slotKey, child.id)} class="text-[var(--muted)] hover:text-[var(--foreground)]" aria-label="Move to top level" title="Move to top level">
				<ArrowUpRight class="h-3 w-3" />
			</button>
			<button type="button" onclick={() => onRemove(containerId, slotKey, child.id)} class="text-[var(--danger)] hover:opacity-80" aria-label="Remove child">
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
					oninput={(e) => onSpanChange(containerId, slotKey, child.id, Number(e.currentTarget.value))}
					class="flex-1 h-1 cursor-pointer"
					style="accent-color: var(--color-primary)"
					aria-label="Column span (1-12)"
				/>
				<!-- Visual 12-col bar -->
				<div class="flex gap-px" style="width: 48px">
					{#each Array(12) as _, j (j)}
						<div
							class="h-2 flex-1 rounded-sm"
							style={j < effectiveSpan(child) ? 'background: var(--color-primary)' : 'background: var(--color-border)'}
						></div>
					{/each}
				</div>
			</div>
		{/if}

		{#if isContainerLike(child)}
			{@const slots = childSlotsOf(child)}
			<div class="ml-2">
				<button
					type="button"
					onclick={() => toggleExpand(child.id)}
					class="text-[10px] text-[var(--muted)] hover:text-[var(--foreground)] px-2 py-0.5"
				>
					{expandedContainers.has(child.id) ? '▼' : '▶'} {slots.reduce((n, s) => n + s.blocks.length, 0)} children
				</button>
			</div>
			{#if expandedContainers.has(child.id)}
				{#each slots as slot (slot.key)}
					{#if slot.label}
						<div class="ml-2 mt-1 px-1 text-[10px] font-medium uppercase tracking-wider text-[var(--muted)]">{slot.label}</div>
					{/if}
					<ContainerChildren
						containerId={child.id}
						slotKey={slot.key}
						children={slot.blocks}
						layout={getContainerLayout(child)}
						columns={getContainerColumns(child)}
						{selectedBlockId}
						{getIcon}
						{expandedContainers}
						{toggleExpand}
						onSelect={onSelect}
						onReorder={onReorder}
						onRemove={onRemove}
						onMoveOut={onMoveOut}
						onSpanChange={onSpanChange}
					/>
				{/each}
			{/if}
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

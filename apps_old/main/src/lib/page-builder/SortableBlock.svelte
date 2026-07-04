<script lang="ts">
	import { createSortable } from '@dnd-kit/svelte/sortable';
	import { GripVertical, Trash2, Copy } from 'lucide-svelte';

	interface Props {
		id: string;
		blockType: string;
		blockLabel: string;
		index: number;
		readOnly: boolean;
		children: import('svelte').Snippet;
		onDelete?: (id: string) => void;
		onDuplicate?: (id: string) => void;
	}

	let {
		id,
		blockType,
		blockLabel,
		index,
		readOnly = false,
		children,
		onDelete,
		onDuplicate,
	}: Props = $props();

	const sortable = createSortable({
		get id() { return id; },
		get index() { return index; },
		get disabled() { return readOnly; },
	});

	let wrapperClass = $derived(
		[
			'group relative rounded-lg border transition-all duration-200',
			readOnly
				? 'border-transparent'
				: 'border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-900',
			!readOnly ? 'hover:border-gray-300 dark:hover:border-gray-600' : '',
			sortable.isDragging ? 'opacity-40' : '',
		]
			.filter(Boolean)
			.join(' ')
	);
</script>

<div
	class={wrapperClass}
	role={!readOnly ? 'listitem' : undefined}
	data-block-id={id}
	data-block-type={blockType}
	{@attach sortable.attach}
>
	{#if !readOnly}
		<!-- Edit-mode toolbar -->
		<div class="flex items-center gap-2 border-b border-gray-100 px-3 py-1.5 dark:border-gray-800">
			<!-- Drag handle -->
			<button
				type="button"
				class="cursor-grab rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600 active:cursor-grabbing dark:hover:bg-gray-800 dark:hover:text-gray-300"
				title="Drag to reorder"
				aria-label="Drag to reorder {blockLabel}"
				{@attach sortable.attachHandle}
			>
				<GripVertical class="h-4 w-4" />
			</button>

			<!-- Block type label -->
			<span class="flex-1 text-xs font-medium text-gray-500 dark:text-gray-400">
				{blockLabel}
			</span>

			<!-- Duplicate button -->
			{#if onDuplicate}
				<button
					type="button"
					class="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-blue-500 dark:hover:bg-gray-800 dark:hover:text-blue-400"
					title="Duplicate block"
					aria-label="Duplicate {blockLabel}"
					onclick={(e) => {
						e.stopPropagation();
						onDuplicate(id);
					}}
				>
					<Copy class="h-3.5 w-3.5" />
				</button>
			{/if}

			<!-- Delete button -->
			{#if onDelete}
				<button
					type="button"
					class="rounded p-1 text-gray-400 hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-950 dark:hover:text-red-400"
					title="Delete block"
					aria-label="Delete {blockLabel}"
					onclick={(e) => {
						e.stopPropagation();
						onDelete(id);
					}}
				>
					<Trash2 class="h-3.5 w-3.5" />
				</button>
			{/if}
		</div>
	{/if}

	<!-- Block content -->
	<div class={!readOnly ? 'p-3' : ''}>
		{@render children()}
	</div>
</div>

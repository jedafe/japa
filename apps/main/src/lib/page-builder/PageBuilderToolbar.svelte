<script lang="ts">
	import { Plus, Eye, EyeOff, Save, Layers } from 'lucide-svelte';

	interface Props {
		blockCount: number;
		isPreview?: boolean;
		onAddBlock: () => void;
		onTogglePreview: () => void;
		onSave: () => void;
	}

	let {
		blockCount,
		isPreview = false,
		onAddBlock,
		onTogglePreview,
		onSave,
	}: Props = $props();

	let previewButtonClass = $derived(
		[
			'inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-gray-50 dark:hover:bg-gray-800',
			isPreview
				? 'border-blue-400 bg-blue-50 text-blue-700 dark:border-blue-500 dark:bg-blue-950 dark:text-blue-300'
				: 'border-gray-200 text-gray-700 dark:border-gray-700 dark:text-gray-300',
		].join(' ')
	);
</script>

<div
	class="flex items-center gap-3 border-b border-gray-200 bg-white px-4 py-2 dark:border-gray-700 dark:bg-gray-900"
	role="toolbar"
	aria-label="Page builder toolbar"
>
	<!-- Add Block -->
	<button
		type="button"
		class="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
		onclick={onAddBlock}
	>
		<Plus class="h-4 w-4" />
		Add Block
	</button>

	<!-- Divider -->
	<div class="h-5 w-px bg-gray-200 dark:bg-gray-700" role="separator"></div>

	<!-- Block count -->
	<span class="flex items-center gap-1.5 text-sm text-gray-500 dark:text-gray-400">
		<Layers class="h-3.5 w-3.5" />
		{blockCount} {blockCount === 1 ? 'block' : 'blocks'}
	</span>

	<!-- Spacer -->
	<div class="flex-1"></div>

	<!-- Preview toggle -->
	<button
		type="button"
		class={previewButtonClass}
		onclick={onTogglePreview}
		aria-pressed={isPreview}
	>
		{#if isPreview}
			<EyeOff class="h-4 w-4" />
			Edit
		{:else}
			<Eye class="h-4 w-4" />
			Preview
		{/if}
	</button>

	<!-- Save -->
	<button
		type="button"
		class="inline-flex items-center gap-1.5 rounded-md bg-gray-900 px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-900 dark:hover:bg-white"
		onclick={onSave}
	>
		<Save class="h-4 w-4" />
		Save
	</button>
</div>

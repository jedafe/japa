<script lang="ts">
	interface Props {
		comparison_id?: string;
		display_mode?: string;
		readOnly?: boolean;
	}

	let {
		comparison_id = '',
		readOnly = false,
	}: Props = $props();
	let wrapperClass = $derived(
		`block-comparison${!readOnly ? ' ring-2 ring-blue-400/40 hover:ring-blue-400/60' : ''}`
	);
</script>

<div class={wrapperClass}>
	{#if comparison_id}
		<div class="flex flex-col items-center justify-center rounded-lg border border-gray-200 bg-gray-50 px-6 py-10 text-center dark:border-gray-700 dark:bg-gray-800/50">
			<svg class="mb-3 h-10 w-10 text-gray-400 dark:text-gray-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
				<path stroke-linecap="round" stroke-linejoin="round" d="M7.5 21L3 16.5m0 0L7.5 12M3 16.5h13.5m0-13.5L21 7.5m0 0L16.5 12M21 7.5H7.5" />
			</svg>
			<p class="text-sm font-medium text-gray-600 dark:text-gray-300">
				Comparison: {comparison_id}
			</p>
			<p class="mt-1 text-xs text-gray-400 dark:text-gray-500">
				Rendered by Section 30 comparison system
			</p>
		</div>
	{:else}
		<div class="flex h-32 items-center justify-center rounded-lg border-2 border-dashed border-gray-300 bg-gray-50 text-sm text-gray-400 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-500">
			Comparison — select a comparison ID
		</div>
	{/if}
</div>

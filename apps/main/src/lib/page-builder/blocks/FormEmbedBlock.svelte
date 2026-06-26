<script lang="ts">
	interface Props {
		form_id?: string;
		show_title?: boolean;
		show_border?: boolean;
		readOnly?: boolean;
	}

	let {
		form_id = '',
		show_title = true,
		show_border = true,
		readOnly = false,
	}: Props = $props();
	let wrapperClass = $derived(
		`block-form-embed${!readOnly ? ' ring-2 ring-blue-400/40 hover:ring-blue-400/60' : ''}`
	);
</script>

<div class={wrapperClass}>
	{#if form_id}
		<div
			class="flex flex-col items-center justify-center rounded-lg px-6 py-10 text-center {show_border ? 'border border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-800/50' : 'bg-transparent'}"
		>
			<svg class="mb-3 h-10 w-10 text-gray-400 dark:text-gray-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
				<path stroke-linecap="round" stroke-linejoin="round" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
			</svg>
			{#if show_title}
				<p class="text-sm font-medium text-gray-600 dark:text-gray-300">
					Form: {form_id}
				</p>
			{/if}
			<p class="mt-1 text-xs text-gray-400 dark:text-gray-500">
				Rendered by Section 06 form system
			</p>
		</div>
	{:else}
		<div class="flex h-32 items-center justify-center rounded-lg border-2 border-dashed border-gray-300 bg-gray-50 text-sm text-gray-400 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-500">
			Form embed — select a form ID
		</div>
	{/if}
</div>

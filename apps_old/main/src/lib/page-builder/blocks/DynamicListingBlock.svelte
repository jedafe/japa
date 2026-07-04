<script lang="ts">
	interface Props {
		content_type?: string;
		space_id?: string;
		limit?: number;
		sort_by?: string;
		sort_order?: string;
		filters?: Record<string, unknown>;
		data_source?: string;
		readOnly?: boolean;
	}

	let {
		content_type = '',
		limit = 10,
		readOnly = false,
	}: Props = $props();
	let wrapperClass = $derived(
		`block-dynamic-listing${!readOnly ? ' ring-2 ring-blue-400/40 hover:ring-blue-400/60' : ''}`
	);
</script>

<div class={wrapperClass}>
	{#if content_type}
		<div class="flex flex-col items-center justify-center rounded-lg border border-gray-200 bg-gray-50 px-6 py-10 text-center dark:border-gray-700 dark:bg-gray-800/50">
			<svg class="mb-3 h-10 w-10 text-gray-400 dark:text-gray-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
				<path stroke-linecap="round" stroke-linejoin="round" d="M8.25 6.75h12M8.25 12h12m-12 5.25h12M3.75 6.75h.007v.008H3.75V6.75zm.375 0a.375.375 0 11-.75 0 .375.375 0 01.75 0zM3.75 12h.007v.008H3.75V12zm.375 0a.375.375 0 11-.75 0 .375.375 0 01.75 0zm-.375 5.25h.007v.008H3.75v-.008zm.375 0a.375.375 0 11-.75 0 .375.375 0 01.75 0z" />
			</svg>
			<p class="text-sm font-medium text-gray-600 dark:text-gray-300">
				Dynamic Listing: {content_type}
			</p>
			<p class="mt-1 text-xs text-gray-400 dark:text-gray-500">
				Showing up to {limit} items — rendered by Section 32
			</p>
		</div>
	{:else}
		<div class="flex h-32 items-center justify-center rounded-lg border-2 border-dashed border-gray-300 bg-gray-50 text-sm text-gray-400 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-500">
			Dynamic listing — select a content type
		</div>
	{/if}
</div>

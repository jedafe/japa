<script lang="ts">
	interface StatItem {
		value: string;
		label: string;
		icon?: string;
	}

	interface Props {
		items?: StatItem[];
		layout?: string;
		readOnly?: boolean;
	}

	let {
		items = [],
		readOnly = false,
	}: Props = $props();
	let wrapperClass = $derived(
		`block-stats${!readOnly ? ' ring-2 ring-blue-400/40 hover:ring-blue-400/60' : ''}`
	);
</script>

<div class={wrapperClass}>
	{#if items.length === 0}
		<div class="flex h-32 items-center justify-center rounded-lg border-2 border-dashed border-gray-300 bg-gray-50 text-sm text-gray-400 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-500">
			No statistics added
		</div>
	{:else}
		<div class="grid grid-cols-2 gap-6 md:grid-cols-3 lg:grid-cols-4">
			{#each items as item}
				<div class="flex flex-col items-center rounded-lg bg-gray-50 p-6 text-center dark:bg-gray-800/50">
					{#if item.icon}
						<span class="mb-2 text-2xl" aria-hidden="true">{item.icon}</span>
					{/if}
					<span class="text-3xl font-bold tracking-tight text-gray-900 dark:text-white md:text-4xl">
						{item.value}
					</span>
					<span class="mt-1 text-sm text-gray-500 dark:text-gray-400">
						{item.label}
					</span>
				</div>
			{/each}
		</div>
	{/if}
</div>

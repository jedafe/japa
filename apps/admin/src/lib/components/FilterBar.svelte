<script lang="ts">
	let {
		filters,
		values: initialValues = {},
		onApply
	}: {
		filters: { key: string; label: string; type?: 'text' | 'select' | 'date'; options?: string[] }[];
		values?: Record<string, string>;
		onApply?: (values: Record<string, string>) => void;
	} = $props();

	let localValues = $state<Record<string, string>>({ ...initialValues });

	function apply() {
		onApply?.(localValues);
	}
</script>

<div class="mb-4 flex flex-wrap items-end gap-3">
	{#each filters as filter}
		<div>
			<label for="filter-{filter.key}" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{filter.label}</label>
			{#if filter.type === 'select'}
				<select
					id="filter-{filter.key}"
					class="rounded-md border px-3 py-1.5 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-surface)"
					bind:value={localValues[filter.key]}
				>
					<option value="">All</option>
					{#each filter.options ?? [] as opt}
						<option value={opt}>{opt}</option>
					{/each}
				</select>
			{:else if filter.type === 'date'}
				<input
					id="filter-{filter.key}"
					type="date"
					class="rounded-md border px-3 py-1.5 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-surface)"
					bind:value={localValues[filter.key]}
				/>
			{:else}
				<input
					id="filter-{filter.key}"
					type="text"
					class="rounded-md border px-3 py-1.5 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-surface)"
					placeholder={filter.label}
					bind:value={localValues[filter.key]}
				/>
			{/if}
		</div>
	{/each}
	<button
		class="rounded-md border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
		style="border-color: var(--color-border)"
		onclick={apply}
	>
		Apply
	</button>
</div>

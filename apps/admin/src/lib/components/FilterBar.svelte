<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { Select } from '$lib/components/ui/select';

	let {
		filters,
		values: initialValues = {},
		onApply
	}: {
		filters: { key: string; label: string; type?: 'text' | 'select' | 'date'; options?: (string | { value: string; label: string })[] }[];
		values?: Record<string, string>;
		onApply?: (values: Record<string, string>) => void;
	} = $props();

	let localValues = $state<Record<string, string>>({});
	
	const allOption = $derived({ value: '', label: t('common.all') });

	$effect(() => {
		const snap = { ...initialValues };
		localValues = snap;
	});

	function apply() {
		onApply?.(localValues);
	}

	function handleSelectChange(key: string, val: string) {
		localValues[key] = val;
	}
</script>

<div class="mb-4 flex flex-wrap items-end gap-3">
	{#each filters as filter (filter)}
		<div class="w-full sm:w-auto">
			<label for="filter-{filter.key}" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{filter.label}</label>
			{#if filter.type === 'select'}
				<Select
					options={[allOption, ...(filter.options ?? []).map(o => typeof o === 'string' ? { value: o, label: o } : o)]}
					value={localValues[filter.key] ?? ''}
					onchange={(val) => handleSelectChange(filter.key, val)}
					class="!h-9"
				/>
			{:else if filter.type === 'date'}
				<input
					id="filter-{filter.key}"
					type="date"
					class="w-full rounded-md border px-3 py-1.5 text-sm sm:w-auto"
					style="border-color: var(--color-border); background-color: var(--color-surface)"
					bind:value={localValues[filter.key]}
				/>
			{:else}
				<input
					id="filter-{filter.key}"
					type="text"
					class="w-full rounded-md border px-3 py-1.5 text-sm sm:w-auto"
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
		{t('common.apply')}
	</button>
</div>

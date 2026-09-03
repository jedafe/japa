<script lang="ts">
	import { t } from '$lib/i18n';
	import { Plus, X } from 'lucide-svelte';
	import Autocomplete from '../Autocomplete.svelte';
	import type { AutocompleteItem } from '../Autocomplete.svelte';
	import type { BlogSeries } from '$lib/types';
	import { searchSeries } from '$lib/api/create';
	import SeriesCreationModal from './SeriesCreationModal.svelte';

	interface Props {
		series: BlogSeries[];
		seriesId: string | null;
		onselect: (id: string | null) => void;
		onfocus?: () => void;
	}

	let { series = $bindable(), seriesId, onselect, onfocus }: Props = $props();

	// The selected series object — used to display the label without a re-fetch.
	const selectedSeries = $derived(series.find((s) => s.id === seriesId) ?? null);

	// Debounced fetcher — calls GET /blog/series?q=… via the API client.
	async function fetcher(q: string): Promise<AutocompleteItem[]> {
		const res = await searchSeries(q);
		return (res.data ?? [])
			.filter((s) => s.id !== seriesId)
			.map((s) => ({
				id: s.id,
				label: s.title,
				sublabel: s.slug
			}));
	}

	// Initial suggestions — show up to 5 series when the field is focused.
	async function initialFetcher(): Promise<AutocompleteItem[]> {
		return series
			.filter((s) => s.id !== seriesId)
			.slice(0, 5)
			.map((s) => ({
				id: s.id,
				label: s.title,
				sublabel: s.slug
			}));
	}

	function handleSelect(item: AutocompleteItem) {
		onselect(item.id);
	}

	function handleClear() {
		onselect(null);
	}

	// ---- "Create new series" modal ----
	let createModalOpen = $state(false);

	function openCreateModal() {
		createModalOpen = true;
	}

	function handleCreated(newSeries: BlogSeries) {
		series = [...series, newSeries];
		onselect(newSeries.id);
	}
</script>

<div class="series-selector">
	<div class="series-selector-row">
		<Autocomplete
			placeholder={t('create.placeholder.series_search')}
			emptyMessage={t('create.placeholder.series_no_results')}
			{fetcher}
			{initialFetcher}
			selectedId={null}
			selectedLabel={null}
			onSelect={handleSelect}
			{onfocus}
		/>
		<button
			type="button"
			class="series-create-btn"
			onclick={openCreateModal}
			title={t('create.series.create_new')}
			aria-label={t('create.series.create_new')}
		>
			<Plus size={16} />
		</button>
	</div>

	{#if selectedSeries}
		<div class="selected-series">
			<span class="series-chip">
				{selectedSeries.title}
				<button
					type="button"
					class="chip-remove"
					onclick={handleClear}
					title={t('create.series.remove', 'Remove')}
					aria-label={t('create.series.remove_aria', `Remove {title}`, { title: selectedSeries.title })}
				>
					<X size={12} />
				</button>
			</span>
		</div>
	{/if}
</div>

<SeriesCreationModal bind:open={createModalOpen} onCreated={handleCreated} />

<style>
	.series-selector {
		display: flex;
		flex-direction: column;
		gap: 8px;
		width: 100%;
	}
	.series-selector-row {
		display: flex;
		gap: 6px;
		align-items: stretch;
	}

	.series-selector-row > :global(*) {
		min-width: 0;
	}

	.series-create-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
		width: 40px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text-subtle);
		cursor: pointer;
		transition: border-color 0.15s, color 0.15s, background 0.15s;
	}
	.series-create-btn:hover {
		border-color: var(--color-primary);
		color: var(--color-primary);
		background: color-mix(in srgb, var(--color-primary) 8%, transparent);
	}

	.selected-series {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
	}
	.series-chip {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		padding: 4px 8px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.75rem;
	}
	.chip-remove {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		padding: 0;
		border: none;
		background: transparent;
		color: var(--color-text-subtle);
		cursor: pointer;
		transition: color 0.15s;
	}
	.chip-remove:hover {
		color: var(--destructive);
	}
</style>

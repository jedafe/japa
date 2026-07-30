<script lang="ts">
	import { t } from '$lib/i18n';
	import { Plus, X, Loader2 } from 'lucide-svelte';
	import Autocomplete from '../Autocomplete.svelte';
	import type { AutocompleteItem } from '../Autocomplete.svelte';
	import type { BlogSeries } from '$lib/types';
	import { searchSeries, createSeries } from '$lib/api/create';
	import { suggestSlug } from '../slug';

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
		return (res.data ?? []).map((s) => ({
			id: s.id,
			label: s.title,
			sublabel: s.slug
		}));
	}

	// Initial suggestions — show up to 5 series when the field is focused.
	async function initialFetcher(): Promise<AutocompleteItem[]> {
		return series
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

	// ---- Inline "Create new series" form ----
	let showCreateForm = $state(false);
	let createTitle = $state('');
	let createSlug = $state('');
	let createDescription = $state('');
	let slugTouched = $state(false);
	let creating = $state(false);
	let createError = $state<string | null>(null);

	function openCreateForm() {
		showCreateForm = true;
		createTitle = '';
		createSlug = '';
		createDescription = '';
		slugTouched = false;
		createError = null;
		queueMicrotask(() => titleInput?.focus());
	}

	function closeCreateForm() {
		showCreateForm = false;
		createError = null;
	}

	function onTitleInput(e: Event) {
		createTitle = (e.currentTarget as HTMLInputElement).value;
		if (!slugTouched) createSlug = suggestSlug(createTitle);
	}

	function onSlugInput(e: Event) {
		createSlug = (e.currentTarget as HTMLInputElement).value;
		slugTouched = true;
	}

	let titleInput = $state<HTMLInputElement | null>(null);

	async function handleCreate() {
		if (creating) return;
		const title = createTitle.trim();
		const slug = createSlug.trim();
		if (!title || !slug) {
			createError = t('create.series.error_fields');
			return;
		}

		creating = true;
		createError = null;
		const res = await createSeries({
			title,
			slug,
			description: createDescription.trim() || undefined
		});
		creating = false;

		if (res.error) {
			createError = res.error.message;
			return;
		}

		// Success — add to local list and select the new series.
		if (res.data) {
			series = [...series, res.data];
			onselect(res.data.id);
			showCreateForm = false;
		}
	}
</script>

{#if showCreateForm}
	<div class="series-create-form">
		<div class="series-create-header">
			<span class="series-create-title">{t('create.series.create_title')}</span>
			<button type="button" class="series-create-close" onclick={closeCreateForm} aria-label={t('common.cancel')}>
				<X size={16} />
			</button>
		</div>

		<div class="series-create-field">
			<label class="series-create-label" for="series-title">{t('create.series.title_label')}</label>
			<input
				id="series-title"
				type="text"
				class="series-create-input"
				placeholder={t('create.series.title_placeholder')}
				value={createTitle}
				oninput={onTitleInput}
				bind:this={titleInput}
				disabled={creating}
			/>
		</div>

		<div class="series-create-field">
			<label class="series-create-label" for="series-slug">{t('create.series.slug_label')}</label>
			<input
				id="series-slug"
				type="text"
				class="series-create-input"
				placeholder={t('create.series.slug_placeholder')}
				value={createSlug}
				oninput={onSlugInput}
				disabled={creating}
			/>
		</div>

		<div class="series-create-field">
			<label class="series-create-label" for="series-desc">{t('create.series.description_label')}</label>
			<input
				id="series-desc"
				type="text"
				class="series-create-input"
				placeholder={t('create.series.description_placeholder')}
				value={createDescription}
				oninput={(e) => (createDescription = e.currentTarget.value)}
				disabled={creating}
			/>
		</div>

		{#if createError}
			<p class="series-create-error">{createError}</p>
		{/if}

		<div class="series-create-actions">
			<button type="button" class="series-create-cancel" onclick={closeCreateForm} disabled={creating}>
				{t('common.cancel')}
			</button>
			<button type="button" class="series-create-submit" onclick={handleCreate} disabled={creating}>
				{#if creating}
					<Loader2 size={14} class="animate-spin" />
					{t('create.series.creating')}
				{:else}
					<Plus size={14} />
					{t('create.series.create_button')}
				{/if}
			</button>
		</div>
	</div>
{:else}
	<div class="series-selector-row">
		<Autocomplete
			placeholder={t('create.placeholder.series_search')}
			emptyMessage={t('create.placeholder.series_no_results')}
			{fetcher}
			{initialFetcher}
			selectedId={seriesId}
			selectedLabel={selectedSeries?.title ?? null}
			onSelect={handleSelect}
			onClear={handleClear}
			{onfocus}
		/>
		<button
			type="button"
			class="series-create-btn"
			onclick={openCreateForm}
			title={t('create.series.create_new')}
			aria-label={t('create.series.create_new')}
		>
			<Plus size={16} />
		</button>
	</div>
{/if}

<style>
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

	/* ---- Inline create form ---- */
	.series-create-form {
		display: flex;
		flex-direction: column;
		gap: 10px;
		padding: 12px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
	}
	.series-create-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}
	.series-create-title {
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--color-text);
	}
	.series-create-close {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 24px;
		height: 24px;
		border: none;
		border-radius: 4px;
		background: transparent;
		color: var(--color-muted);
		cursor: pointer;
		transition: background 0.1s, color 0.1s;
	}
	.series-create-close:hover {
		background: var(--color-surface-subtle);
		color: var(--color-text);
	}

	.series-create-field {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.series-create-label {
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--color-muted);
	}
	.series-create-input {
		width: 100%;
		padding: 8px 10px;
		border: 1px solid var(--color-border);
		border-radius: calc(var(--radius) - 2px);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.875rem;
		outline: none;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.series-create-input::placeholder {
		color: var(--color-muted);
	}
	.series-create-input:focus {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 2px color-mix(in srgb, var(--color-primary) 15%, transparent);
	}

	.series-create-error {
		margin: 0;
		font-size: 0.75rem;
		color: var(--color-danger, #dc2626);
	}

	.series-create-actions {
		display: flex;
		gap: 8px;
		justify-content: flex-end;
	}
	.series-create-cancel {
		padding: 6px 12px;
		border: 1px solid var(--color-border);
		border-radius: calc(var(--radius) - 2px);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.8125rem;
		cursor: pointer;
		transition: background 0.1s;
	}
	.series-create-cancel:hover:not(:disabled) {
		background: var(--color-surface-subtle);
	}
	.series-create-submit {
		display: flex;
		align-items: center;
		gap: 4px;
		padding: 6px 12px;
		border: none;
		border-radius: calc(var(--radius) - 2px);
		background: var(--color-primary);
		color: var(--color-primary-contrast, var(--color-surface));
		font-size: 0.8125rem;
		font-weight: 500;
		cursor: pointer;
		transition: background 0.1s;
	}
	.series-create-submit:hover:not(:disabled) {
		background: var(--color-primary-hover);
	}
	.series-create-submit:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
</style>

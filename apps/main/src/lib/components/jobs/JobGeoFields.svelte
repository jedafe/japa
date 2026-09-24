<script lang="ts">
	// Cascading Country → State → City pickers for the job listing form,
	// backed by the geo-space catalog (GET /geo/countries,
	// /geo/countries/{code}/states, /geo/states/{id}/cities). Only the
	// country and city NAMES are stored in field_values — the jobs finder
	// matches `country` by name and `city` is display-only. The state step
	// exists purely to navigate the hierarchy to the city list.
	//
	// Ref: japaconnect-engineering/sections/63-jobs-board.md
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { listCountries, listStates, listCities, createCity } from '$lib/api/geo';
	import type { GeoCountry, GeoState, GeoCity } from '$lib/types';

	interface Props {
		values: Record<string, unknown>;
		onvalues: (v: Record<string, unknown>) => void;
		onedit: () => void;
	}

	let { values, onvalues, onedit }: Props = $props();

	let countries = $state<GeoCountry[] | null>(null);
	let countryCode = $state('');
	let states = $state<GeoState[]>([]);
	let stateId = $state('');
	let cities = $state<GeoCity[]>([]);

	const countryName = $derived(typeof values.country === 'string' ? values.country : '');
	const cityName = $derived(typeof values.city === 'string' ? values.city : '');
	const selectedCountryLabel = $derived(
		countryName ? (countries?.find((c) => c.name === countryName)?.name ?? countryName) : null
	);
	const selectedStateLabel = $derived(states.find((s) => s.id === stateId)?.name ?? null);

	function setField(key: string, val: string) {
		onvalues({ ...values, [key]: val });
		onedit();
	}

	$effect(() => {
		if (countries === null) {
			countries = [];
			listCountries()
				.then((list) => {
					countries = list;
					// Edit prefill: the stored value is the country name — resolve it
					// back to a geo_code so the state cascade can load.
					const existing = untrack(() => countryName);
					const match = existing ? list.find((c) => c.name === existing) : undefined;
					if (match) {
						countryCode = match.geo_code;
						void loadStates(match.geo_code);
					}
				})
				.catch(() => (countries = []));
		}
	});

	async function loadStates(code: string) {
		states = [];
		cities = [];
		stateId = '';
		if (!code) return;
		states = await listStates(code);
	}

	async function loadCities(sid: string) {
		cities = [];
		if (!sid) return;
		cities = await listCities(sid);
	}

	function filterItems<T extends { name: string; geo_code?: string | null }>(
		list: T[],
		q: string,
		map: (item: T) => AutocompleteItem
	): AutocompleteItem[] {
		const needle = q.toLowerCase().trim();
		const filtered = needle
			? list.filter(
					(i) =>
						i.name.toLowerCase().includes(needle) ||
						(i.geo_code ?? '').toLowerCase().includes(needle)
				)
			: list;
		return filtered.slice(0, 50).map(map);
	}

	const countryFetcher = (q: string): Promise<AutocompleteItem[]> =>
		Promise.resolve(
			filterItems(countries ?? [], q, (c) => ({ id: c.geo_code, label: c.name, sublabel: c.geo_code }))
		);
	const stateFetcher = (q: string): Promise<AutocompleteItem[]> =>
		Promise.resolve(filterItems(states, q, (s) => ({ id: s.id, label: s.name })));
	const cityFetcher = (q: string): Promise<AutocompleteItem[]> =>
		Promise.resolve(filterItems(cities, q, (c) => ({ id: c.id, label: c.name })));

	async function addCity(query: string): Promise<AutocompleteItem | null> {
		if (!stateId || !query.trim()) return null;
		const created = await createCity(stateId, query);
		if (!created) return null;
		cities = [...cities, created];
		return { id: created.id, label: created.name };
	}
</script>

<div class="geo-fields">
	<div class="geo-field">
		<label class="geo-label" for="job-country">{t('jobs.country', 'Country')}</label>
		<Autocomplete
			id="job-country"
			placeholder={t('jobs.country', 'Country')}
			emptyMessage={t('common.no_results', 'No results found')}
			fetcher={countryFetcher}
			initialFetcher={() => countryFetcher('')}
			selectedId={countryCode || null}
			selectedLabel={selectedCountryLabel}
			onSelect={(item) => {
				const picked = (countries ?? []).find((c) => c.geo_code === item.id);
				countryCode = item.id;
				setField('country', picked?.name ?? item.label);
				setField('city', '');
				void loadStates(item.id);
			}}
			onClear={() => {
				countryCode = '';
				states = [];
				cities = [];
				stateId = '';
				setField('country', '');
				setField('city', '');
			}}
		/>
	</div>

	{#if countryCode && states.length > 0}
		<div class="geo-field">
			<label class="geo-label" for="job-state">{t('jobs.state', 'State / Region')}</label>
			<Autocomplete
				id="job-state"
				placeholder={t('jobs.state', 'State / Region')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={stateFetcher}
				initialFetcher={() => stateFetcher('')}
				selectedId={stateId || null}
				selectedLabel={selectedStateLabel}
				onSelect={(item) => {
					stateId = item.id;
					setField('city', '');
					void loadCities(item.id);
				}}
				onClear={() => {
					stateId = '';
					cities = [];
					setField('city', '');
				}}
			/>
		</div>
	{/if}

	{#if countryCode}
		<div class="geo-field">
			<label class="geo-label" for="job-city">{t('jobs.city', 'City')}</label>
			{#if states.length > 0 && !stateId}
				<p class="geo-hint">{t('jobs.city_pick_state', 'Select a state / region first')}</p>
			{:else if stateId}
				<Autocomplete
					id="job-city"
					placeholder={t('jobs.city', 'City')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={cityFetcher}
					initialFetcher={() => cityFetcher('')}
					selectedId={cities.find((c) => c.name === cityName)?.id ?? null}
					selectedLabel={cityName || null}
					addActionLabel={t('jobs.city_add', 'Add "{query}"')}
					onAdd={addCity}
					onSelect={(item) => setField('city', item.label)}
					onClear={() => setField('city', '')}
				/>
			{:else}
				<!-- Country has no seeded states — free-text fallback. -->
				<input
					id="job-city"
					type="text"
					class="geo-input"
					placeholder={t('jobs.city', 'City')}
					value={cityName}
					oninput={(e) => setField('city', e.currentTarget.value)}
				/>
			{/if}
		</div>
	{/if}
</div>

<style>
	.geo-fields {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
		gap: 12px;
	}
	.geo-field {
		display: flex;
		flex-direction: column;
		gap: 4px;
		min-width: 0;
	}
	.geo-label {
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--color-muted);
	}
	.geo-hint {
		font-size: 0.8125rem;
		color: var(--color-muted);
		padding: 8px 0;
	}
	.geo-input {
		width: 100%;
		height: 2.375rem;
		padding: 0 14px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.875rem;
		outline: none;
	}
	.geo-input:focus {
		border-color: var(--color-primary);
	}
</style>

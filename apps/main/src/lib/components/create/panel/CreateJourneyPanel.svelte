<script lang="ts">
	// Ref: japaconnect-engineering/sections/32-activity-feed-timeline.md
	// Ref: 2026-08-21-journey-aware-recommendations-design.md
	// Rule 16: countries come from /geo/countries. These fields describe the
	// journey this CONTENT covers — never auto-filled from the author's own
	// context (§14 Global Template Literals: an author can write about
	// destinations unrelated to their own journey).
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { getJourneyOptions } from '$lib/api/journey';
	import type { JourneyOptions, JourneyOption } from '$lib/api/journey';
	import type { GeoCountry } from '$lib/types';
	import Autocomplete from '../Autocomplete.svelte';
	import type { AutocompleteItem } from '../Autocomplete.svelte';

	interface Props {
		journeyStage: string | null;
		visaRoute: string | null;
		sourceCountry: string | null;
		targetCountry: string | null;
		dimension: string | null;
		purpose: string | null;
		assessmentDimensions: string[];
		onjourney: (v: string | null) => void;
		onvisa: (v: string | null) => void;
		onsource: (v: string | null) => void;
		ontarget: (v: string | null) => void;
		ondimension: (v: string | null) => void;
		onpurpose: (v: string | null) => void;
		onedit: () => void;
	}

	let {
		journeyStage,
		visaRoute,
		sourceCountry,
		targetCountry,
		dimension,
		purpose,
		assessmentDimensions,
		onjourney,
		onvisa,
		onsource,
		ontarget,
		ondimension,
		onpurpose,
		onedit
	}: Props = $props();

	let loaded = $state(false);
	let countryList = $state<GeoCountry[]>([]);
	let options = $state<JourneyOptions | null>(null);
	let stageOptions = $derived(options?.stages ?? []);
	let visaOptions = $derived(options?.visa_routes ?? []);
	let dimensionOptions = $derived(assessmentDimensions.map((d) => ({ value: d, label: d })));
	let purposeOptions = $derived(options?.purposes ?? []);

	$effect(() => {
		if (loaded) return;
		loaded = true;
		void (async () => {
			// Fetch independently so one failed request does not block the other.
			const [countriesRes, optsRes] = await Promise.allSettled([
				api.get<GeoCountry[]>('/geo/countries'),
				getJourneyOptions()
			]);
			if (countriesRes.status === 'fulfilled' && countriesRes.value.data) {
				countryList = countriesRes.value.data;
			}
			if (optsRes.status === 'fulfilled' && optsRes.value.data) {
				options = optsRes.value.data;
			}
		})();
	});

	function countryItems(): AutocompleteItem[] {
		return countryList.map((c) => ({ id: c.geo_code, label: c.name, sublabel: c.geo_code }));
	}

	function countryFetcher(q: string): Promise<AutocompleteItem[]> {
		const needle = q.toLowerCase().trim();
		const filtered = needle
			? countryList.filter(
					(c) => c.name.toLowerCase().includes(needle) || c.geo_code.toLowerCase().includes(needle)
				)
			: countryList;
		return Promise.resolve(filtered.slice(0, 50).map((c) => ({ id: c.geo_code, label: c.name, sublabel: c.geo_code })));
	}

	function countryInitialFetcher(): Promise<AutocompleteItem[]> {
		return Promise.resolve(countryItems().slice(0, 8));
	}

	function countryLabel(code: string): string {
		const c = countryList.find((x) => x.geo_code === code);
		return c?.name ?? code;
	}

	function countryInitial(value: string | null): Promise<AutocompleteItem[]> {
		if (!value) return countryInitialFetcher();
		const c = countryList.find((x) => x.geo_code === value);
		return Promise.resolve([{ id: value, label: c?.name ?? value, sublabel: value }]);
	}

	function optionItems(list: JourneyOption[]): AutocompleteItem[] {
		return list.map((o) => ({ id: o.value, label: o.label, sublabel: o.value }));
	}

	function optionFetcher(list: JourneyOption[]): (q: string) => Promise<AutocompleteItem[]> {
		return (q: string) => {
			const needle = q.toLowerCase().trim();
			const filtered = needle
				? list.filter(
						(o) =>
							o.label.toLowerCase().includes(needle) ||
							o.value.toLowerCase().includes(needle)
					)
				: list;
			return Promise.resolve(optionItems(filtered.slice(0, 20)));
		};
	}

	function optionInitial(list: JourneyOption[], value: string | null): Promise<AutocompleteItem[]> {
		if (!value) return Promise.resolve(optionItems(list.slice(0, 8)));
		const o = list.find((x) => x.value === value);
		return Promise.resolve([{ id: value, label: o?.label ?? value, sublabel: value }]);
	}

	function optionLabel(list: JourneyOption[], value: string): string {
		const o = list.find((x) => x.value === value);
		return o?.label ?? value;
	}

	function setJourney(item: AutocompleteItem) {
		onjourney(item.id);
		onedit();
	}

	function setVisa(item: AutocompleteItem) {
		onvisa(item.id);
		onedit();
	}

	function setJourneyText(v: string) {
		const trimmed = v.trim() || null;
		onjourney(trimmed);
		onedit();
	}

	function setVisaText(v: string) {
		const trimmed = v.trim() || null;
		onvisa(trimmed);
		onedit();
	}

	function setSource(item: AutocompleteItem | null) {
		onsource(item?.id ?? null);
		onedit();
	}

	function setTarget(item: AutocompleteItem | null) {
		ontarget(item?.id ?? null);
		onedit();
	}

	function setDimension(item: AutocompleteItem) {
		ondimension(item.id);
		onedit();
	}

	function setPurpose(item: AutocompleteItem) {
		onpurpose(item.id);
		onedit();
	}
</script>

<div class="journey-panel">
	<div class="field-row">
		<label class="field-label" for="journey-stage">{t('create.journey.stage')}</label>
		{#if stageOptions.length > 0}
			<Autocomplete
				placeholder={t('create.journey.stage_placeholder')}
				emptyMessage={t('journey.banner.no_options', 'No matching option')}
				fetcher={optionFetcher(stageOptions)}
				initialFetcher={() => optionInitial(stageOptions, journeyStage)}
				selectedId={journeyStage}
				selectedLabel={optionLabel(stageOptions, journeyStage ?? '')}
				onSelect={setJourney}
				onClear={() => { onjourney(null); onedit(); }}
			/>
		{:else}
			<input
				id="journey-stage"
				type="text"
				class="field-input"
				placeholder={t('create.journey.stage_placeholder')}
				value={journeyStage ?? ''}
				oninput={(e) => setJourneyText(e.currentTarget.value)}
			/>
		{/if}
	</div>

	<div class="field-row">
		<label class="field-label" for="journey-visa">{t('create.journey.visa')}</label>
		{#if visaOptions.length > 0}
			<Autocomplete
				placeholder={t('create.journey.visa_placeholder')}
				emptyMessage={t('journey.banner.no_options', 'No matching option')}
				fetcher={optionFetcher(visaOptions)}
				initialFetcher={() => optionInitial(visaOptions, visaRoute)}
				selectedId={visaRoute}
				selectedLabel={optionLabel(visaOptions, visaRoute ?? '')}
				onSelect={setVisa}
				onClear={() => { onvisa(null); onedit(); }}
				allowFreeText
			/>
		{:else}
			<input
				id="journey-visa"
				type="text"
				class="field-input"
				placeholder={t('create.journey.visa_placeholder')}
				value={visaRoute ?? ''}
				oninput={(e) => setVisaText(e.currentTarget.value)}
			/>
		{/if}
	</div>

	<div class="field-row">
		<label class="field-label" for="journey-source">{t('create.journey.source_country')}</label>
		<Autocomplete
			placeholder={t('create.journey.source_country_placeholder')}
			emptyMessage={t('create.journey.country_empty')}
			fetcher={countryFetcher}
			initialFetcher={() => countryInitial(sourceCountry)}
			selectedId={sourceCountry}
			selectedLabel={countryLabel(sourceCountry ?? '')}
			onSelect={setSource}
			onClear={() => setSource(null)}
		/>
	</div>

	<div class="field-row">
		<label class="field-label" for="journey-target">{t('create.journey.target_country')}</label>
		<Autocomplete
			placeholder={t('create.journey.target_country_placeholder')}
			emptyMessage={t('create.journey.country_empty')}
			fetcher={countryFetcher}
			initialFetcher={() => countryInitial(targetCountry)}
			selectedId={targetCountry}
			selectedLabel={countryLabel(targetCountry ?? '')}
			onSelect={setTarget}
			onClear={() => setTarget(null)}
		/>
	</div>

	<div class="field-row">
		<label class="field-label" for="journey-dimension">{t('create.journey.dimension', 'Roadmap dimension')}</label>
		<Autocomplete
			id="journey-dimension"
			placeholder={t('create.journey.dimension_placeholder', 'Select a readiness dimension')}
			emptyMessage={t('journey.banner.no_options', 'No matching option')}
			fetcher={optionFetcher(dimensionOptions)}
			initialFetcher={() => optionInitial(dimensionOptions, dimension)}
			selectedId={dimension}
			selectedLabel={optionLabel(dimensionOptions, dimension ?? '')}
			onSelect={setDimension}
			onClear={() => { ondimension(null); onedit(); }}
		/>
	</div>

	{#if dimension === null || dimension === ''}
		<p class="journey-hint">{t('create.journey.dimension_nudge', 'Set a roadmap dimension so this guide can appear in readiness roadmaps.')}</p>
	{/if}

	<div class="field-row">
		<label class="field-label" for="journey-purpose">{t('create.journey.purpose', 'Purpose')}</label>
		{#if purposeOptions.length > 0}
			<Autocomplete
				id="journey-purpose"
				placeholder={t('create.journey.purpose_placeholder', 'Select a purpose')}
				emptyMessage={t('journey.banner.no_options', 'No matching option')}
				fetcher={optionFetcher(purposeOptions)}
				initialFetcher={() => optionInitial(purposeOptions, purpose)}
				selectedId={purpose}
				selectedLabel={optionLabel(purposeOptions, purpose ?? '')}
				onSelect={setPurpose}
				onClear={() => { onpurpose(null); onedit(); }}
			/>
		{:else}
			<input
				id="journey-purpose"
				type="text"
				class="field-input"
				placeholder={t('create.journey.purpose_placeholder', 'Select a purpose')}
				value={purpose ?? ''}
				oninput={(e) => { onpurpose(e.currentTarget.value.trim() || null); onedit(); }}
			/>
		{/if}
	</div>

	<p class="journey-hint">{t('create.journey.hint')}</p>
</div>

<style>
	.journey-panel {
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding: 0.5rem;
	}

	.field-row {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.field-label {
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--color-muted);
	}

	.field-input {
		width: 100%;
		height: 2.375rem;
		padding: 0 14px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.875rem;
		outline: none;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.field-input::placeholder {
		color: var(--color-muted);
	}
	.field-input:focus {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 15%, transparent);
	}

	.journey-hint {
		font-size: 0.75rem;
		color: var(--color-muted);
		line-height: 1.4;
	}
</style>

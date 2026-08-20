<script lang="ts">
	// Ref: japaconnect-engineering/sections/32-activity-feed-timeline.md
	// Ref: 2026-08-21-journey-aware-recommendations-design.md
	// Rule 16: countries come from /geo/countries; user journey comes from /feed/context.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import type { GeoCountry, VisitorContextSummary } from '$lib/types';
	import Autocomplete from '../Autocomplete.svelte';
	import type { AutocompleteItem } from '../Autocomplete.svelte';

	interface Props {
		journeyStage: string | null;
		visaRoute: string | null;
		sourceCountry: string | null;
		targetCountry: string | null;
		onjourney: (v: string | null) => void;
		onvisa: (v: string | null) => void;
		onsource: (v: string | null) => void;
		ontarget: (v: string | null) => void;
		onedit: () => void;
	}

	let {
		journeyStage,
		visaRoute,
		sourceCountry,
		targetCountry,
		onjourney,
		onvisa,
		onsource,
		ontarget,
		onedit
	}: Props = $props();

	let loaded = $state(false);
	let countryList = $state<GeoCountry[]>([]);

	$effect(() => {
		if (loaded) return;
		loaded = true;
		void (async () => {
			const [countriesRes, ctxRes] = await Promise.all([
				api.get<GeoCountry[]>('/geo/countries'),
				api.get<VisitorContextSummary>('/feed/context')
			]);
			if (countriesRes.data) {
				countryList = countriesRes.data;
			}
			if (ctxRes.data) {
				// Pre-fill from the user's resolved journey context only when
				// the content doesn't already carry journey metadata. The
				// panel should not overwrite explicit author choices.
				if (!journeyStage && ctxRes.data.journey_stage) onjourney(ctxRes.data.journey_stage);
				if (!visaRoute && ctxRes.data.visa_route) onvisa(ctxRes.data.visa_route);
				if (!sourceCountry && ctxRes.data.origin_country) onsource(ctxRes.data.origin_country);
				if (!targetCountry && ctxRes.data.destination_country) ontarget(ctxRes.data.destination_country);
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

	function setJourney(v: string) {
		const trimmed = v.trim() || null;
		onjourney(trimmed);
		onedit();
	}

	function setVisa(v: string) {
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
</script>

<div class="journey-panel">
	<div class="field-row">
		<label class="field-label" for="journey-stage">{t('create.journey.stage')}</label>
		<input
			id="journey-stage"
			type="text"
			class="field-input"
			placeholder={t('create.journey.stage_placeholder')}
			value={journeyStage ?? ''}
			onchange={(e) => setJourney(e.currentTarget.value)}
		/>
	</div>

	<div class="field-row">
		<label class="field-label" for="journey-visa">{t('create.journey.visa')}</label>
		<input
			id="journey-visa"
			type="text"
			class="field-input"
			placeholder={t('create.journey.visa_placeholder')}
			value={visaRoute ?? ''}
			onchange={(e) => setVisa(e.currentTarget.value)}
		/>
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
		padding: 10px 14px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 1rem;
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

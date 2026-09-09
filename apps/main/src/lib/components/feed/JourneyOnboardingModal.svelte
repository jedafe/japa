<script lang="ts">
	// Ref: 2026-08-21-journey-aware-recommendations-design.md §6.1
	// Guest journey onboarding modal. Appears automatically after 5 minutes
	// when a guest has not set their destination, and can be opened manually
	// via the JourneyContextBar.
	import { browser } from '$app/environment';
	import { goto } from '$app/navigation';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { getFeedContext, getJourneyOptions, setFeedContext } from '$lib/api/journey';
	import { journeyModal, JOURNEY_MODAL_DELAY_MS } from '$lib/stores/journey.svelte';
	import type { GeoCountry, VisitorContextSummary } from '$lib/types';
	import type { JourneyOptions, JourneyOption } from '$lib/api/journey';
	import Autocomplete from '../create/Autocomplete.svelte';
	import type { AutocompleteItem } from '../create/Autocomplete.svelte';
	import { Dialog } from 'bits-ui';

	let loaded = $state(false);
	let countryList = $state<GeoCountry[]>([]);
	let ctx = $state<VisitorContextSummary | null>(null);
	let options = $state<JourneyOptions | null>(null);
	let saving = $state(false);
	let error = $state<string | null>(null);
	let timer: ReturnType<typeof setTimeout> | null = null;

	let origin = $state('');
	let destination = $state('');
	let japaStage = $state('');
	let visaRoute = $state('');
	let purpose = $state('');
	let travelDate = $state('');

	$effect(() => {
		if (loaded) return;
		loaded = true;
		void (async () => {
			const [countriesRes, ctxRes, optsRes] = await Promise.all([
				api.get<GeoCountry[]>('/geo/countries'),
				getFeedContext(),
				getJourneyOptions()
			]);
			if (countriesRes.data) {
				countryList = countriesRes.data;
			}
			if (optsRes.data) {
				options = optsRes.data;
			}
			if (ctxRes.data) {
				ctx = ctxRes.data;
				origin = ctxRes.data.origin_country ?? '';
				destination = ctxRes.data.destination_country ?? '';
				japaStage = ctxRes.data.journey_stage ?? '';
				visaRoute = ctxRes.data.visa_route ?? '';
				purpose = ctxRes.data.purpose ?? '';
				travelDate = ctxRes.data.travel_date ?? '';
			}

			if (browser && isDefaultContext(ctx)) {
				timer = setTimeout(() => {
					journeyModal.open = true;
				}, JOURNEY_MODAL_DELAY_MS);
			}
		})();

		return () => {
			if (timer) clearTimeout(timer);
		};
	});

	function isDefaultContext(c: VisitorContextSummary | null): boolean {
		if (!c) return true;
		if (c.is_authenticated) return false;
		return !c.destination_country;
	}

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

	function countryInitialFetcher(value: string): () => Promise<AutocompleteItem[]> {
		return () => {
			if (!value) return Promise.resolve(countryItems().slice(0, 8));
			const c = countryList.find((x) => x.geo_code === value);
			return Promise.resolve([{ id: value, label: c?.name ?? value, sublabel: value }]);
		};
	}

	function countryLabel(code: string): string {
		const c = countryList.find((x) => x.geo_code === code);
		return c?.name ?? code;
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

	function optionInitialFetcher(list: JourneyOption[], value: string): () => Promise<AutocompleteItem[]> {
		return () => {
			if (!value) return Promise.resolve(optionItems(list.slice(0, 8)));
			const c = list.find((o) => o.value === value);
			return Promise.resolve([{ id: value, label: c?.label ?? value }]);
		};
	}

	function optionLabel(list: JourneyOption[], value: string): string {
		const o = list.find((x) => x.value === value);
		return o?.label ?? value;
	}

	function hasOptions(list: JourneyOption[] | undefined): list is JourneyOption[] {
		return !!list && list.length > 0;
	}

	async function submit() {
		if (!origin.trim() || !destination.trim()) {
			error = t('journey.banner.error_required', 'Please select an origin and destination country.');
			return;
		}
		saving = true;
		error = null;
		try {
			const res = await setFeedContext({
				origin_country: origin,
				destination_country: destination,
				japa_stage: japaStage,
				visa_route: visaRoute,
				purpose: purpose,
				travel_date: travelDate
			});
			if (res.error) {
				error = res.error.message;
				return;
			}
			journeyModal.open = false;
			if (browser) {
				// Re-run server loaders so the feed uses the new journey context.
				goto(`${window.location.pathname}${window.location.search}`, { invalidateAll: true });
			}
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			saving = false;
		}
	}
</script>

<Dialog.Root bind:open={journeyModal.open}>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 z-40 bg-black/50" />
		<Dialog.Content
			class="fixed z-50 max-h-[90dvh] overflow-y-auto rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 shadow-xl sm:left-1/2 sm:top-1/2 sm:w-[calc(100%-2rem)] sm:max-w-xl sm:-translate-x-1/2 sm:-translate-y-1/2 max-sm:bottom-0 max-sm:left-0 max-sm:right-0 max-sm:top-auto max-sm:w-full max-sm:max-w-none max-sm:translate-x-0 max-sm:translate-y-0 max-sm:rounded-t-2xl max-sm:rounded-b-none max-sm:p-4"
		>
			<div class="flex items-start justify-between gap-3">
				<div>
					<Dialog.Title class="text-lg font-semibold text-[var(--color-text)]">
						{t('journey.banner.title', 'Tell us about your journey')}
					</Dialog.Title>
					<Dialog.Description class="mt-0.5 text-sm text-[var(--color-text-subtle)]">
						{t('journey.banner.subtitle', "We'll show content that matches where you're coming from and where you're going.")}
					</Dialog.Description>
				</div>
				<Dialog.Close
					class="shrink-0 text-[var(--color-muted)] hover:text-[var(--color-text)]"
					aria-label={t('common.close', 'Close')}
				>
					<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>
				</Dialog.Close>
			</div>

			{#if error}
				<p class="mt-4 rounded bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-950 dark:text-red-200">{error}</p>
			{/if}

			<div class="mt-4 grid gap-4 sm:grid-cols-2">
				<div class="field-row">
					<label class="field-label" for="journey-origin">{t('journey.banner.origin_country', 'Where are you now?')}</label>
					<Autocomplete
						placeholder={t('journey.banner.origin_placeholder', 'e.g. Nigeria')}
						emptyMessage={t('create.journey.country_empty', 'No country found')}
						fetcher={countryFetcher}
						initialFetcher={countryInitialFetcher(origin)}
						selectedId={origin}
						selectedLabel={countryLabel(origin)}
						onSelect={(item) => { origin = item.id; }}
						onClear={() => { origin = ''; }}
					/>
				</div>

				<div class="field-row">
					<label class="field-label" for="journey-destination">{t('journey.banner.destination_country', 'Where do you want to go?')}</label>
					<Autocomplete
						placeholder={t('journey.banner.destination_placeholder', 'e.g. Canada')}
						emptyMessage={t('create.journey.country_empty', 'No country found')}
						fetcher={countryFetcher}
						initialFetcher={countryInitialFetcher(destination)}
						selectedId={destination}
						selectedLabel={countryLabel(destination)}
						onSelect={(item) => { destination = item.id; }}
						onClear={() => { destination = ''; }}
					/>
				</div>

				<div class="field-row">
					<label class="field-label" for="journey-purpose">{t('journey.banner.purpose', 'Why are you traveling?')}</label>
					{#if hasOptions(options?.purposes)}
						<Autocomplete
							placeholder={t('journey.banner.purpose_placeholder', 'e.g. study')}
							emptyMessage={t('journey.banner.no_options', 'No matching option')}
							fetcher={optionFetcher(options.purposes)}
							initialFetcher={optionInitialFetcher(options.purposes, purpose)}
							selectedId={purpose}
							selectedLabel={optionLabel(options.purposes, purpose)}
							onSelect={(item) => { purpose = item.id; }}
							onClear={() => { purpose = ''; }}
							allowFreeText
						/>
					{:else}
						<input
							id="journey-purpose"
							type="text"
							class="field-input"
							placeholder={t('journey.banner.purpose_placeholder', 'e.g. study')}
							bind:value={purpose}
						/>
					{/if}
				</div>

				<div class="field-row">
					<label class="field-label" for="journey-visa">{t('journey.banner.visa_route', 'Which visa route?')}</label>
					{#if hasOptions(options?.visa_routes)}
						<Autocomplete
							placeholder={t('journey.banner.visa_placeholder', 'e.g. student')}
							emptyMessage={t('journey.banner.no_options', 'No matching option')}
							fetcher={optionFetcher(options.visa_routes)}
							initialFetcher={optionInitialFetcher(options.visa_routes, visaRoute)}
							selectedId={visaRoute}
							selectedLabel={optionLabel(options.visa_routes, visaRoute)}
							onSelect={(item) => { visaRoute = item.id; }}
							onClear={() => { visaRoute = ''; }}
							allowFreeText
						/>
					{:else}
						<input
							id="journey-visa"
							type="text"
							class="field-input"
							placeholder={t('journey.banner.visa_placeholder', 'e.g. student')}
							bind:value={visaRoute}
						/>
					{/if}
				</div>

				<div class="field-row">
					<label class="field-label" for="journey-stage">{t('journey.banner.stage', 'Where are you in the journey?')}</label>
					{#if hasOptions(options?.stages)}
						<Autocomplete
							placeholder={t('journey.banner.stage_placeholder', 'e.g. preparing')}
							emptyMessage={t('journey.banner.no_options', 'No matching option')}
							fetcher={optionFetcher(options.stages)}
							initialFetcher={optionInitialFetcher(options.stages, japaStage)}
							selectedId={japaStage}
							selectedLabel={optionLabel(options.stages, japaStage)}
							onSelect={(item) => { japaStage = item.id; }}
							onClear={() => { japaStage = ''; }}
						/>
					{:else}
						<input
							id="journey-stage"
							type="text"
							class="field-input"
							placeholder={t('journey.banner.stage_placeholder', 'e.g. preparing')}
							bind:value={japaStage}
						/>
					{/if}
				</div>

				<div class="field-row">
					<label class="field-label" for="journey-travel-date">{t('journey.banner.travel_date', 'Planned travel date (optional)')}</label>
					<input
						id="journey-travel-date"
						type="date"
						class="field-input"
						bind:value={travelDate}
					/>
				</div>
			</div>

			<div class="mt-6 flex items-center justify-end gap-3">
				<Dialog.Close
					type="button"
					class="text-sm text-[var(--color-muted)] hover:text-[var(--color-text)]"
				>
					{t('common.close', 'Close')}
				</Dialog.Close>
				<button
					type="button"
					class="rounded bg-[var(--color-primary)] px-4 py-2 text-sm font-semibold text-[var(--color-primary-contrast)] hover:opacity-90 disabled:opacity-50"
					onclick={submit}
					disabled={saving}
				>
					{saving ? t('journey.banner.saving', 'Saving...') : t('journey.banner.submit', 'Show me relevant content')}
				</button>
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<style>
	.field-row {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}
	.field-label {
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.025em;
		color: var(--color-text-subtle);
	}
	.field-input {
		width: 100%;
		border-radius: var(--radius);
		border: 1px solid var(--color-border);
		background-color: var(--color-surface);
		padding: 0.5rem 0.75rem;
		font-size: 0.875rem;
		color: var(--color-text);
	}
	.field-input::placeholder {
		color: var(--color-muted);
	}
	.field-input:focus {
		outline: 2px solid var(--color-primary);
		outline-offset: -2px;
	}
</style>

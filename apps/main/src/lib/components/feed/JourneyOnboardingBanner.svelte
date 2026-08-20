<script lang="ts">
	// Ref: 2026-08-21-journey-aware-recommendations-design.md §6.1
	// Rule 16: countries come from /geo/countries; canonical values are
	// normalized on the backend, so free-text inputs are used for
	// purpose/visa/stage (same pattern as CreateJourneyPanel).
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { getFeedContext, setFeedContext } from '$lib/api/journey';
	import type { GeoCountry, VisitorContextSummary } from '$lib/types';
	import Autocomplete from '../create/Autocomplete.svelte';
	import type { AutocompleteItem } from '../create/Autocomplete.svelte';
	import { X } from 'lucide-svelte';

	interface Props {
		open?: boolean;
		onOpenChange?: (open: boolean) => void;
		onComplete?: () => void;
	}

	let { open = $bindable(true), onOpenChange, onComplete }: Props = $props();

	let loaded = $state(false);
	let countryList = $state<GeoCountry[]>([]);
	let ctx = $state<VisitorContextSummary | null>(null);
	let saving = $state(false);
	let error = $state<string | null>(null);

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
			const [countriesRes, ctxRes] = await Promise.all([
				api.get<GeoCountry[]>('/geo/countries'),
				getFeedContext()
			]);
			if (countriesRes.data) {
				countryList = countriesRes.data;
			}
			if (ctxRes.data) {
				ctx = ctxRes.data;
				origin = ctxRes.data.origin_country ?? '';
				destination = ctxRes.data.destination_country ?? '';
				japaStage = ctxRes.data.journey_stage ?? ctxRes.data.japa_stage ?? '';
				visaRoute = ctxRes.data.visa_route ?? '';
				purpose = ctxRes.data.purpose ?? '';
				travelDate = ctxRes.data.travel_date ?? '';
			}
		})();
	});

	function setOpen(value: boolean) {
		open = value;
		onOpenChange?.(value);
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

	function isDefaultContext(c: VisitorContextSummary | null): boolean {
		if (!c) return true;
		if (c.is_authenticated) return false;
		return !c.origin_country && !c.destination_country;
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
			// The backend set an HttpOnly cookie. Reload so the next SSR/hydrate
			// cycle resolves the new context and the banner flips to the bar.
			onComplete?.();
			if (typeof window !== 'undefined') {
				window.location.reload();
			}
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			saving = false;
		}
	}
</script>

{#if open && (isDefaultContext(ctx) || !ctx)}
	<div class="mx-auto w-full max-w-full">
		<div class="border-b border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-4 py-4">
			<div class="flex items-start justify-between gap-3">
				<div>
					<h2 class="text-base font-semibold text-[var(--color-text)]">{t('journey.banner.title', "Tell us about your journey")}</h2>
					<p class="mt-0.5 text-sm text-[var(--color-text-subtle)]">{t('journey.banner.subtitle', "We'll show content that matches where you're coming from and where you're going.")}</p>
				</div>
				<button
					type="button"
					class="shrink-0 text-[var(--color-muted)] hover:text-[var(--color-text)]"
					onclick={() => setOpen(false)}
					aria-label={t('common.close', 'Close')}
				>
					<X size={18} />
				</button>
			</div>

			{#if error}
				<p class="mt-3 rounded bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-950 dark:text-red-200">{error}</p>
			{/if}

			<div class="mt-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
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
					<input
						id="journey-purpose"
						type="text"
						class="field-input"
						placeholder={t('journey.banner.purpose_placeholder', 'e.g. study')}
						bind:value={purpose}
					/>
				</div>

				<div class="field-row">
					<label class="field-label" for="journey-visa">{t('journey.banner.visa_route', 'Which visa route?')}</label>
					<input
						id="journey-visa"
						type="text"
						class="field-input"
						placeholder={t('journey.banner.visa_placeholder', 'e.g. student')}
						bind:value={visaRoute}
					/>
				</div>

				<div class="field-row">
					<label class="field-label" for="journey-stage">{t('journey.banner.stage', 'Where are you in the journey?')}</label>
					<input
						id="journey-stage"
						type="text"
						class="field-input"
						placeholder={t('journey.banner.stage_placeholder', 'e.g. preparing')}
						bind:value={japaStage}
					/>
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

			<div class="mt-4 flex items-center gap-3">
				<button
					type="button"
					class="rounded bg-[var(--color-primary)] px-4 py-2 text-sm font-semibold text-[var(--color-primary-contrast)] hover:opacity-90 disabled:opacity-50"
					onclick={submit}
					disabled={saving}
				>
					{saving ? t('journey.banner.saving', 'Saving...') : t('journey.banner.submit', 'Show me relevant content')}
				</button>
				<button
					type="button"
					class="text-sm text-[var(--color-muted)] hover:text-[var(--color-text)]"
					onclick={() => setOpen(false)}
				>
					{t('common.close', 'Close')}
				</button>
			</div>
		</div>
	</div>
{/if}

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

<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	// Ref: 2026-08-21-journey-aware-recommendations-design.md §6.1
	// Guest journey onboarding modal. Appears automatically after 1 minute
	// when a guest has not set their destination, and can be opened manually
	// via the JourneyContextBar.
	import { browser } from '$app/environment';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { destinationMarkets, originMarkets, pinMarketsFirst } from '$lib/utils/geoMarkets';
	import { getFeedContextCached, getGeoCountriesCached, getJourneyOptionsCached, setFeedContext } from '$lib/api/journey';
	import { journeyModal, JOURNEY_MODAL_DELAY_MS } from '$lib/stores/journey.svelte';
	import type { GeoCountry, VisitorContextSummary } from '$lib/types';
	import type { JourneyOptions, JourneyOption } from '$lib/api/journey';
	import Autocomplete from '../create/Autocomplete.svelte';
	import type { AutocompleteItem } from '../create/Autocomplete.svelte';
	import { Modal } from '$lib/components/ui';
	import ThemeLogo from '$lib/components/branding/ThemeLogo.svelte';
	import { Compass, Flag, Heart, CircleCheck, ShieldCheck, Plane, GraduationCap, Briefcase, MapPin, Globe, ChevronRight, X } from 'lucide-svelte';

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

	// Multi-step wizard state (4 steps matching the header stepper).
	let step = $state(1);
	const steps = $derived([
		{ icon: Compass, label: t('journey.step_journey', 'Your Journey') },
		{ icon: Flag, label: t('journey.step_goals', 'Your Goals') },
		{ icon: Heart, label: t('journey.step_preferences', 'Preferences') },
		{ icon: CircleCheck, label: t('journey.step_done', "You're All Set") }
	]);
	const canContinue = $derived(step !== 1 || (origin.trim() !== '' && destination.trim() !== ''));

	function next() {
		if (!canContinue) {
			error = t('journey.banner.error_required', 'Please select an origin and destination country.');
			return;
		}
		error = null;
		if (step < 4) step++;
		else submit();
	}
	function back() {
		error = null;
		if (step > 1) step--;
	}

	$effect(() => {
		if (loaded) return;
		loaded = true;
		void (async () => {
			const [countriesRes, ctxRes, optsRes] = await Promise.all([
				getGeoCountriesCached(),
				getFeedContextCached(),
				getJourneyOptionsCached()
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

	// ADR-064 curated markets — pinned first in the pickers; every seeded
	// country stays selectable (pin-first, not restricted).
	const originPins = $derived(originMarkets(page.data.siteSettings));
	const destinationPins = $derived(destinationMarkets(page.data.siteSettings));

	// Same brand logo as the app shell: appearance.logo_url(_dark) with the
	// ThemeLogo wordmark fallback — never the square icon mark.
	const logoUrl = $derived((page.data.siteSettings as Record<string, string> | undefined)?.['appearance.logo_url'] ?? '');
	const logoDarkUrl = $derived((page.data.siteSettings as Record<string, string> | undefined)?.['appearance.logo_url_dark'] ?? '');

	function countryItems(list: GeoCountry[] = countryList): AutocompleteItem[] {
		return list.map((c) => ({ id: c.geo_code, label: c.name, sublabel: c.geo_code }));
	}

	function countryFetcherFor(markets: string[]): (q: string) => Promise<AutocompleteItem[]> {
		return (q: string) => {
			const needle = q.toLowerCase().trim();
			const ordered = pinMarketsFirst(countryList, markets, (c) => c.geo_code);
			const filtered = needle
				? ordered.filter(
						(c) => c.name.toLowerCase().includes(needle) || c.geo_code.toLowerCase().includes(needle)
					)
				: ordered;
			return Promise.resolve(countryItems(filtered.slice(0, 50)));
		};
	}

	function countryInitialFetcherFor(markets: string[], value: string): () => Promise<AutocompleteItem[]> {
		return () => {
			if (!value)
				return Promise.resolve(
					countryItems(pinMarketsFirst(countryList, markets, (c) => c.geo_code)).slice(0, 8)
				);
			const c = countryList.find((x) => x.geo_code === value);
			return Promise.resolve([{ id: value, label: c?.name ?? value, sublabel: value }]);
		};
	}

	function countryLabel(code: string): string {
		const c = countryList.find((x) => x.geo_code === code);
		return c?.name ?? code;
	}

	function optionItems(list: JourneyOption[]): AutocompleteItem[] {
		// No sublabel — echoing the raw value would render as "Study study".
		return list.map((o) => ({ id: o.value, label: o.label }));
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
			step = 1;
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

<Modal
	bind:open={journeyModal.open}
	onOpenChange={(v) => { if (!v) step = 1; }}
	hideHeader
	padded={false}
	ariaTitle={t('journey.banner.title', 'Tell us about your journey')}
	closeLabel={t('common.close', 'Close')}
	size="max-w-4xl"
	class="max-sm:bottom-0 max-sm:start-0 max-sm:end-0 max-sm:top-auto max-sm:w-full max-sm:max-w-none max-sm:translate-x-0 max-sm:translate-y-0 max-sm:rounded-t-2xl max-sm:rounded-b-none"
>
	{#snippet children(close)}
		<!-- Header: brand + stepper + close -->
		<div class="flex items-center gap-4 border-b border-[var(--color-border)] px-5 py-4 sm:px-7">
			<ThemeLogo {logoUrl} {logoDarkUrl} alt={t('app.name', '{site}')} class="h-6 w-auto shrink-0" />
			<nav aria-label={t('journey.steps_label', 'Journey setup steps')} class="mx-auto flex items-center">
				{#each steps as s, i (i)}
					{@const n = i + 1}
					<button
						type="button"
						class="group flex flex-col items-center gap-1.5 disabled:cursor-default"
						disabled={n >= step}
						onclick={() => { error = null; step = n; }}
						aria-current={n === step ? 'step' : undefined}
					>
						<span
							class="grid h-9 w-9 place-items-center rounded-full transition-colors {n === step
								? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
								: n < step
									? 'bg-[var(--color-surface-subtle)] text-[var(--color-primary)]'
									: 'bg-[var(--color-surface-subtle)] text-[var(--color-muted)]'}"
						>
							<s.icon size={16} />
						</span>
						<span class="hidden text-[11px] font-medium sm:block {n === step ? 'text-[var(--color-primary)]' : 'text-[var(--color-muted)]'}">{s.label}</span>
					</button>
					{#if i < steps.length - 1}
						<span class="mx-2 mb-0 h-px w-8 sm:mb-5 sm:w-12 {n < step ? 'bg-[var(--color-primary)]' : 'bg-[var(--color-border)]'}"></span>
					{/if}
				{/each}
			</nav>
			<button
				type="button"
				onclick={close}
				aria-label={t('common.close', 'Close')}
				class="grid h-8 w-8 shrink-0 place-items-center rounded-full text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
			>
				<X size={16} />
			</button>
		</div>

		<div class="grid grid-cols-1 sm:grid-cols-[230px_1fr]">
			<!-- Decorative panel — CSS illustration: flight route + floating chips. -->
			<aside aria-hidden="true" class="journey-scene relative hidden overflow-hidden sm:block">
				<svg class="journey-arc" viewBox="0 0 200 220" fill="none">
					<path d="M30 200 C 30 120, 90 90, 160 40" stroke="var(--color-primary)" stroke-width="1.5" stroke-dasharray="5 6" stroke-linecap="round" opacity="0.45" />
					<circle cx="30" cy="200" r="4" fill="var(--color-primary)" opacity="0.5" />
					<circle cx="160" cy="40" r="4" fill="var(--color-primary)" opacity="0.5" />
				</svg>
				<div class="journey-globe">
					<Globe size={56} strokeWidth={1.2} class="text-[var(--color-primary)]" />
				</div>
				<div class="journey-chip" style="top: 14%; inset-inline-start: 10%;">
					<GraduationCap size={18} class="text-[var(--color-primary)]" />
				</div>
				<div class="journey-chip" style="top: 44%; inset-inline-start: 6%;">
					<Briefcase size={18} class="text-[var(--color-primary)]" />
				</div>
				<div class="journey-chip" style="bottom: 16%; inset-inline-end: 12%;">
					<MapPin size={18} class="text-[var(--color-primary)]" />
				</div>
				<div class="journey-plane">
					<Plane size={20} class="text-[var(--color-primary)]" />
				</div>
			</aside>

			<!-- Step content -->
			<div class="p-6 sm:p-8">
				{#if step === 1}
					<h2 class="text-xl font-bold text-[var(--color-text)]">{t('journey.banner.title', 'Tell us about your journey')}</h2>
					<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{t('journey.banner.subtitle', "We'll show content that matches where you're coming from and where you're going.")}</p>

					<div class="mt-6 grid grid-cols-1 gap-4 sm:grid-cols-2">
						<div class="field-row">
							<label class="field-label" for="journey-origin">{t('journey.banner.origin_country', 'Where are you now?')}
							<HintIcon text={t('journey.banner.origin_hint', 'The country you currently live in.')}  />
</label>
							<Autocomplete
								placeholder={t('journey.banner.origin_placeholder', 'e.g. Nigeria')}
								emptyMessage={t('create.journey.country_empty', 'No country found')}
								fetcher={countryFetcherFor(originPins)}
								initialFetcher={countryInitialFetcherFor(originPins, origin)}
								selectedId={origin}
								selectedLabel={countryLabel(origin)}
								onSelect={(item) => { origin = item.id; }}
								onClear={() => { origin = ''; }}
							/>
						</div>
						<div class="field-row">
							<label class="field-label" for="journey-destination">{t('journey.banner.destination_country', 'Where do you want to go?')}
							<HintIcon text={t('journey.banner.destination_hint', 'The country you plan to move to — drives your checklist.')}  />
</label>
							<Autocomplete
								placeholder={t('journey.banner.destination_placeholder', 'e.g. Canada')}
								emptyMessage={t('create.journey.country_empty', 'No country found')}
								fetcher={countryFetcherFor(destinationPins)}
								initialFetcher={countryInitialFetcherFor(destinationPins, destination)}
								selectedId={destination}
								selectedLabel={countryLabel(destination)}
								onSelect={(item) => { destination = item.id; }}
								onClear={() => { destination = ''; }}
							/>
						</div>
					</div>
				{:else if step === 2}
					<h2 class="text-xl font-bold text-[var(--color-text)]">{t('journey.step2_title', 'What are your goals?')}</h2>
					<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{t('journey.step2_subtitle', 'Tell us why you are moving so we can surface the right opportunities.')}</p>

					<div class="mt-6 grid grid-cols-1 gap-4 sm:grid-cols-2">
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
								<input id="journey-purpose" type="text" class="field-input" placeholder={t('journey.banner.purpose_placeholder', 'e.g. study')} bind:value={purpose} aria-describedby="journey-purpose-hint" />
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
								<input id="journey-visa" type="text" class="field-input" placeholder={t('journey.banner.visa_placeholder', 'e.g. student')} bind:value={visaRoute} aria-describedby="journey-visa-hint" />
								<HintIcon id="journey-visa-hint" text={t('journey.banner.visa_hint', 'The visa category you are applying for, if kn>{t('journey.step3_title', 'A few more details')}</h2>
					<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{t('journey.step3_subtitle', 'Optional — helps us time the content we show you.')}</p>

					<div class="mt-6 grid grid-cols-1 gap-4 sm:grid-cols-2">
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
								<input id="journey-stage" type="text" class="field-input" placeholder={t('journey.banner.stage_placeholder', 'e.g. preparing')} bind:value={japaStage} aria-describedby="journey-stage-hint" />
								<HintIcon id="journey-stage-hint" text={t('journey.banner.stage_hint', 'E.g. researching, preparing documents, waiting on a decision.')} />
							{/if}
						</div>
						<div class="field-row">
							<label class="field-label" for="journey-travel-date">{t('journey.banner.travel_date', 'Planned travel date (optional)')}
								<HintIcon id="journey-travel-date-hint" text={t('journey.banner.travel_date_hint', 'Your planned departure — used to time checklists and reminders.')} /></label>
							<input id="journey-travel-date" type="date" class="field-input" bind:value={travelDate} aria-describedby="journey-travel-date-hint" />
								<HintIcon id="journey-travel-date-hint" text={t('journey.banner.travel_date_hint', 'Your planned departure — used to time checklists and reminders.')} />
						</div>
					</div>
				{:else}
					<h2 class="text-xl font-bold text-[var(--color-text)]y details — you can change them anytime in your profile.')}</p>

					<dl class="mt-6 grid grid-cols-1 gap-x-6 gap-y-3 sm:grid-cols-2">
						<div class="summary-row"><dt>{t('journey.banner.origin_country', 'Where are you now?')}</dt><dd>{countryLabel(origin)}</dd></div>
						<div class="summary-row"><dt>{t('journey.banner.destination_country', 'Where do you want to go?')}</dt><dd>{countryLabel(destination)}</dd></div>
						{#if purpose}<div class="summary-row"><dt>{t('journey.banner.purpose', 'Why are you traveling?')}</dt><dd>{optionLabel(options?.purposes ?? [], purpose)}</dd></div>{/if}
						{#if visaRoute}<div class="summary-row"><dt>{t('journey.banner.visa_route', 'Which visa route?')}</dt><dd>{optionLabel(options?.visa_routes ?? [], visaRoute)}</dd></div>{/if}
						{#if japaStage}<div class="summary-row"><dt>{t('journey.banner.stage', 'Where are you in the journey?')}</dt><dd>{optionLabel(options?.stages ?? [], japaStage)}</dd></div>{/if}
						{#if travelDate}<div class="summary-row"><dt>{t('journey.banner.travel_date', 'Planned travel date (optional)')}</dt><dd>{travelDate}</dd></div>{/if}
					</dl>
				{/if}

				{#if error}
					<p class="mt-4 rounded bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-950 dark:text-red-200">{error}</p>
				{/if}

				<div class="mt-6 flex items-start gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-4 py-3">
					<ShieldCheck size={20} class="mt-0.5 shrink-0 text-[var(--color-primary)]" />
					<p class="text-xs leading-relaxed text-[var(--color-text-subtle)]">
						<span class="font-semibold text-[var(--color-text)]">{t('journey.privacy_title', 'Your information helps us personalize your experience.')}</span>
						{t('journey.privacy_body', 'You can update these details anytime in your profile.')}
					</p>
				</div>

				<div class="mt-6 flex items-center justify-between gap-3">
					<button
						type="button"
						onclick={step === 1 ? close : back}
						class="rounded-full border border-[var(--color-border)] px-5 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						{step === 1 ? t('common.close', 'Close') : t('common.back', 'Back')}
					</button>
					<button
						type="button"
						class="flex items-center gap-2 rounded-full bg-[var(--color-primary)] px-5 py-2 text-sm font-semibold text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover,var(--color-primary))] disabled:opacity-50"
						onclick={next}
						disabled={saving}
					>
						{step === 4
							? (saving ? t('journey.banner.saving', 'Saving...') : t('journey.banner.submit', 'Show me relevant content'))
							: t('common.continue', 'Continue')}
						<ChevronRight class="rtl:-scale-x-100" size={15} />
					</button>
				</div>
			</div>
		</div>
	{/snippet}
</Modal>

<style>
	.journey-scene {
		background:
			radial-gradient(ellipse 75% 55% at 30% 45%, color-mix(in srgb, var(--color-primary) 12%, transparent), transparent),
			color-mix(in srgb, var(--color-primary) 5%, var(--color-surface));
	}
	.journey-arc {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
	}
	.journey-globe {
		position: absolute;
		top: 50%;
		inset-inline-start: 50%;
		translate: -50% -50%;
		display: grid;
		place-items: center;
		width: 7rem;
		height: 7rem;
		border-radius: 9999px;
		background: var(--color-surface);
		box-shadow: 0 8px 24px color-mix(in srgb, var(--color-primary) 15%, transparent);
	}
	.journey-chip {
		position: absolute;
		display: grid;
		place-items: center;
		width: 2.5rem;
		height: 2.5rem;
		border-radius: 0.75rem;
		background: var(--color-surface);
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
	}
	.journey-plane {
		position: absolute;
		top: 10%;
		inset-inline-end: 16%;
		color: var(--color-primary);
		opacity: 0.8;
	}
	@media (prefers-reduced-motion: no-preference) {
		.journey-chip {
			animation: journey-float 4s ease-in-out infinite;
		}
		.journey-chip:nth-of-type(2) { animation-delay: 0.7s; }
		.journey-chip:nth-of-type(3) { animation-delay: 1.4s; }
		.journey-plane {
			animation: journey-float 3.5s ease-in-out infinite;
		}
	}
	@keyframes journey-float {
		0%, 100% { translate: 0 0; }
		50% { translate: 0 -6px; }
	}
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
	.summary-row {
		display: flex;
		flex-direction: column;
		gap: 0.125rem;
	}
	.summary-row dt {
		font-size: 0.7rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.025em;
		color: var(--color-muted);
	}
	.summary-row dd {
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--color-text);
	}
</style>

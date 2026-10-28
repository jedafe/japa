<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	import { untrack } from 'svelte';
	import { goto, invalidateAll } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { navigatePostLogin } from '$lib/auth/redirect';
	import { t } from '$lib/i18n';
	import { Check, ChevronLeft, ChevronRight, Sparkles, MapPin, Trophy, Heart, ShieldCheck, Plane, Globe, Landmark, Luggage, Lightbulb } from 'lucide-svelte';
	import { destinationMarkets, pinMarketsFirst, servedMarkets } from '$lib/utils/geoMarkets';
	import ThemeLogo from '$lib/components/branding/ThemeLogo.svelte';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import type { PageData } from './$types';
	import type { GeoCity, GeoState } from '$lib/types';

	let { data }: { data: PageData } = $props();

	// Start at the backend's current step (or step 1).
	let currentStep = $state(untrack(() => data.status?.current_step ?? 1));
	let saving = $state(false);
	let error = $state<string | null>(null);

	// Onboarding minimums from system_settings (onboarding.min_tags_follow etc.)
	// exposed via GET /site/settings/public. Ref: conventions/system-settings-catalog.md
	const minTags = $derived(Number(data.siteSettings?.['onboarding.min_tags_follow'] ?? 3));
	const minUsers = $derived(Number(data.siteSettings?.['onboarding.min_users_follow'] ?? 2));
	const minSpaces = $derived(Number(data.siteSettings?.['onboarding.min_spaces_join'] ?? 2));

	// Step 1: Core Identity — pre-fill location from GeoIP (ADR-037).
	let name = $state('');
	let japaStatus = $state('');
	let currentLocation = $state(
		untrack(() => {
			const g = data.geoContext;
			if (g?.is_valid && g.city) return `${g.city}, ${g.country_name}`;
			if (g?.is_valid && g.country_name) return g.country_name;
			return '';
		})
	);
	// Country selector sourced from /geo/countries (ADR-005). Empty = no
	// selection (user can still free-text the location field).
	let selectedCountry = $state(untrack(() => data.geoContext?.country_code ?? ''));
	let selectedState = $state('');
	let selectedCity = $state('');
	let states = $state<GeoState[]>([]);
	let cities = $state<GeoCity[]>([]);
	let loadingStates = $state(false);
	let loadingCities = $state(false);

	// Step 2: Destination & Goals
	// `destination` stores the country NAME (the backend persists it as a
	// free-text string in onboarding step_data). `destCountryCode` holds the
	// selected ISO geo_code from the Autocomplete. The picker is sourced from
	// the spaces country table (data.countries, loaded from /geo/countries)
	// so the user selects a real seeded country — never a hardcoded list.
	let destination = $state('');
	let destCountryCode = $state('');
	let primaryGoal = $state('');

	// ADR-064 curated markets (public settings via data.siteSettings) —
	// pinned first in the country pickers; every seeded country stays
	// selectable (pin-first, not restricted).
	const destPins = $derived(destinationMarkets(data.siteSettings));
	const locationPins = $derived(servedMarkets(data.siteSettings));

	// Same brand logo as the app shell: appearance.logo_url(_dark) with the
	// ThemeLogo wordmark fallback — never the square icon mark.
	const logoUrl = $derived((data.siteSettings as Record<string, string> | undefined)?.['appearance.logo_url'] ?? '');
	const logoDarkUrl = $derived((data.siteSettings as Record<string, string> | undefined)?.['appearance.logo_url_dark'] ?? '');

	// Destination country autocomplete — filters the SSR-loaded country
	// spaces client-side (same pattern as LocationSection.svelte).
	function destCountryFetcher(q: string): Promise<AutocompleteItem[]> {
		const needle = q.toLowerCase().trim();
		const list = pinMarketsFirst(data.countries ?? [], destPins, (c) => c.geo_code);
		const filtered = needle
			? list.filter(
					(c) => c.name.toLowerCase().includes(needle) || c.geo_code.toLowerCase().includes(needle)
				)
			: list;
		return Promise.resolve(
			filtered.slice(0, 50).map((c) => ({ id: c.geo_code, label: c.name, sublabel: c.geo_code }))
		);
	}

	function destCountryInitialFetcher(): Promise<AutocompleteItem[]> {
		const list = pinMarketsFirst(data.countries ?? [], destPins, (c) => c.geo_code);
		return Promise.resolve(
			list.slice(0, 8).map((c) => ({ id: c.geo_code, label: c.name, sublabel: c.geo_code }))
		);
	}

	function destCountryLabel(id: string): string {
		const c = (data.countries ?? []).find((c) => c.geo_code === id);
		return c?.name ?? id;
	}

	// Step 1: home-country autocomplete — keyed by ISO geo_code.
	function fetchStep1Countries(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase().trim();
		const list = pinMarketsFirst(data.countries ?? [], locationPins, (c) => c.geo_code);
		const filtered = query === ''
			? list
			: list.filter(
					(c) => c.name.toLowerCase().includes(query) || c.geo_code.toLowerCase().includes(query)
				);
		return Promise.resolve(filtered.map((c) => ({ id: c.geo_code, label: c.name, sublabel: c.geo_code })));
	}
	function fetchStep1CountriesInitial(): Promise<AutocompleteItem[]> {
		return fetchStep1Countries('');
	}
	const selectedCountryLabel = $derived(
		selectedCountry ? data.countries.find((c) => c.geo_code === selectedCountry)?.name ?? selectedCountry : null
	);

	// Step 3: Interests/Tags
	let selectedTags = $state<string[]>([]);

	// Steps 4–6: per-type follow steps — users, spaces, sources (Section 09).
	let selectedUsers = $state<string[]>([]);
	let selectedSpaces = $state<string[]>([]);
	let selectedSources = $state<string[]>([]);

	const userRecs = $derived(data.recommendations.filter((r) => r.type === 'user'));
	const spaceRecs = $derived(data.recommendations.filter((r) => r.type === 'space'));

	// Section 09: per-step gating. Steps 1 and 2 are mandatory; steps 3–5 must
	// meet their admin-configurable minimums; step 6 (sources) has no minimum;
	// step 7 is the terminal finish screen.
	const canAdvance = $derived(
		currentStep === 1
			? name.trim() !== '' && japaStatus !== '' && currentLocation.trim() !== ''
			: currentStep === 2
				? destCountryCode !== '' && primaryGoal !== ''
				: currentStep === 3
					? selectedTags.length >= minTags
					: currentStep === 4
						? selectedUsers.length >= minUsers
						: currentStep === 5
							? selectedSpaces.length >= minSpaces
							: true
	);

	const totalSteps = 7;
	const stepLabels = $derived([
		t('onboarding.step1', 'Welcome'),
		t('onboarding.step2', 'Destination'),
		t('onboarding.step3', 'Interests'),
		t('onboarding.step4', 'People'),
		t('onboarding.step5', 'Spaces'),
		t('onboarding.step6', 'Sources'),
		t('onboarding.step7', 'Get Started')
	]);

	function toggleTag(tagSlug: string) {
		selectedTags = selectedTags.includes(tagSlug)
			? selectedTags.filter((s) => s !== tagSlug)
			: [...selectedTags, tagSlug];
	}

	function toggleIn(list: string[], id: string): string[] {
		return list.includes(id) ? list.filter((s) => s !== id) : [...list, id];
	}

	// Cascade: fetch states when country changes.
	$effect(() => {
		const code = selectedCountry;
		if (!code) {
			states = [];
			selectedState = '';
			cities = [];
			selectedCity = '';
			return;
		}
		loadingStates = true;
		states = [];
		selectedState = '';
		cities = [];
		selectedCity = '';
		api.get<GeoState[]>(`/geo/countries/${code}/states`).then((res) => {
			if (res.data) states = res.data;
		}).finally(() => { loadingStates = false; });
	});

	// Cascade: fetch cities when state changes.
	$effect(() => {
		const id = selectedState;
		if (!id) {
			cities = [];
			selectedCity = '';
			return;
		}
		loadingCities = true;
		cities = [];
		selectedCity = '';
		api.get<GeoCity[]>(`/geo/states/${id}/cities`).then((res) => {
			if (res.data) cities = res.data;
		}).finally(() => { loadingCities = false; });
	});

	async function saveStep() {
		saving = true;
		error = null;

		const stepData: Record<string, unknown> = {};
		if (currentStep === 1) {
			stepData.name = name;
			stepData.japa_status = japaStatus;
			stepData.current_location = currentLocation;
			stepData.country_code = selectedCountry;
			stepData.state_id = selectedState;
			stepData.city_id = selectedCity;
		} else if (currentStep === 2) {
			stepData.destination = destination;
			// ADR-064a: Also send the ISO geo_code so the backend can save
			// it as the `destination_country` custom field, feeding the
			// geo-aware personalization feature without requiring the user
			// to set it again in Settings.
			stepData.destination_country_code = destCountryCode;
			stepData.primary_goal = primaryGoal;
		} else if (currentStep === 3) {
			// Send both 'tags' and 'interests' so the backend recommendation
			// engine (which reads 'tags' with 'interests' fallback) works.
			stepData.tags = selectedTags;
			stepData.interests = selectedTags;
		} else if (currentStep === 4) {
			stepData.follows = selectedUsers;
		} else if (currentStep === 5) {
			stepData.follows = selectedSpaces;
		} else if (currentStep === 6) {
			stepData.source_follows = selectedSources;
		}

		try {
			if (currentStep < totalSteps) {
				const res = await api.put('/onboarding/step', { step: currentStep, step_data: stepData });
				if (res.error) {
					error = res.error.message;
					saving = false;
					return;
				}
				currentStep++;
			} else {
				// Final step — complete onboarding.
				const res = await api.post('/onboarding/complete', {});
				if (res.error) {
					error = res.error.message;
					saving = false;
					return;
				}
				// Section 09 Product Tour: redirect to home with ?tour=1 so the
				// app shell can trigger the contextual walkthrough after it
				// renders the tour targets (primary nav, space switcher, compose).
				// invalidateAll refreshes root-layout onboardingCompleted so the
				// finish-setup card disappears immediately after completion.
				// §120: a ?redirect= target (e.g. the MCP OAuth authorize URL)
				// supersedes the tour — navigatePostLogin handles the absolute
				// API-host URL via location.assign.
				await invalidateAll();
				const dest = data.postOnboardingRedirect ?? '/';
				if (dest === '/') {
					await goto('/?tour=1');
				} else {
					await navigatePostLogin(dest);
				}
			}
		} catch {
			error = t('onboarding.error', 'Something went wrong. Please try again.');
		}
		saving = false;
	}

	async function skipStep() {
		saving = true;
		error = null;
		try {
			if (currentStep < totalSteps) {
				// Section 09 skip semantics: send skip:true so the backend
				// auto-fulfills any step minimum using the ranked algorithm
				// rather than bypassing the requirement.
				const res = await api.put('/onboarding/step', { step: currentStep, step_data: {}, skip: true });
				if (res.error) {
					error = res.error.message;
					saving = false;
					return;
				}
				currentStep++;
			} else {
				// Terminal-step "skip" is the same as finishing: complete
				// onboarding and redirect to the home feed with the product
				// tour.
				await saveStep();
			}
		} catch {
			error = t('onboarding.error', 'Something went wrong. Please try again.');
		}
		saving = false;
	}

	// §89 Achievement Feed preview — milestone label helper for the final step.
	function milestoneLabel(m?: string) {
		if (!m) return '';
		return t(`achievements.type_${m}`, m.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase()));
	}
</script>

<svelte:head>
	<title>{t('onboarding.title', 'Welcome to {site}')} — {t('app.name')}</title>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div class="min-h-screen bg-[var(--color-bg)] px-4 py-6 sm:px-6 sm:py-10">
	<div class="mx-auto grid grid-cols-1 w-full max-w-5xl overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] shadow-xl lg:grid-cols-[360px_1fr]">
		<!-- Brand panel — decorative illustration + privacy note. -->
		<aside aria-hidden="true" class="onb-side relative hidden flex-col gap-4 overflow-hidden p-8 lg:flex xl:p-10">
			<ThemeLogo {logoUrl} {logoDarkUrl} alt={t('app.name', '{site}')} class="h-7 w-auto self-start" />
			<h2 class="text-[1.4rem] leading-snug font-bold text-[var(--color-text)]">
				{t('onboarding.side_title', "Let's personalize your Japa journey")} <span aria-hidden="true">👋</span>
			</h2>
			<p class="text-sm leading-relaxed text-[var(--color-text-subtle)]">
				{t('onboarding.side_subtitle', 'Tell us a bit about yourself so we can recommend the right opportunities, resources, and tools for your goals.')}
			</p>

			<!-- CSS illustration: flight route + landmark chips. -->
			<div class="relative mt-2 min-h-52 flex-1">
				<svg class="absolute inset-0 h-full w-full" viewBox="0 0 300 220" fill="none" aria-hidden="true">
					<path d="M30 195 C 60 110, 140 120, 255 45" stroke="var(--color-primary)" stroke-width="1.5" stroke-dasharray="5 6" stroke-linecap="round" opacity="0.45" />
					<circle cx="30" cy="195" r="4.5" fill="var(--color-primary)" opacity="0.5" />
					<circle cx="255" cy="45" r="4.5" fill="var(--color-primary)" opacity="0.5" />
				</svg>
				<div class="onb-focal">
					<Globe size={64} strokeWidth={1.1} class="text-[var(--color-primary)]" />
				</div>
				<div class="onb-chip" style="top: 8%; inset-inline-start: 12%;"><Landmark size={18} class="text-[var(--color-primary)]" /></div>
				<div class="onb-chip" style="top: 30%; inset-inline-end: 8%;"><Luggage size={18} class="text-[var(--color-primary)]" /></div>
				<div class="onb-chip" style="bottom: 22%; inset-inline-start: 6%;"><MapPin size={18} class="text-[var(--color-primary)]" /></div>
				<div class="onb-plane"><Plane size={20} class="text-[var(--color-primary)]" /></div>
			</div>

			<div class="mt-auto flex items-start gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)]/80 px-4 py-3 backdrop-blur-sm">
				<ShieldCheck size={18} class="mt-0.5 shrink-0 text-[var(--color-primary)]" />
				<p class="text-xs leading-relaxed text-[var(--color-text-subtle)]">
					<span class="block font-semibold text-[var(--color-text)]">{t('onboarding.privacy_title', 'Your data is safe with us.')}</span>
					{t('onboarding.privacy_body', 'We respect your privacy and never share your data.')}
				</p>
			</div>
		</aside>

		<div class="flex min-w-0 flex-col p-6 sm:p-10">
			<!-- Numbered stepper -->
			<nav aria-label={t('onboarding.steps_label', 'Onboarding steps')} class="mb-8 flex items-start">
				{#each stepLabels as label, i (i)}
					{@const n = i + 1}
					<div class="flex flex-1 flex-col items-center gap-1.5 last:flex-none">
						<div class="flex w-full items-center">
							{#if i > 0}<span class="h-px flex-1 {n <= currentStep ? 'bg-[var(--color-primary)]' : 'bg-[var(--color-border)]'}"></span>{/if}
							<span
								class="grid h-9 w-9 shrink-0 place-items-center rounded-full text-sm font-semibold transition-colors {n === currentStep
									? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
									: n < currentStep
										? 'bg-[var(--color-primary)]/15 text-[var(--color-primary)]'
										: 'bg-[var(--color-surface-subtle)] text-[var(--color-muted)]'}"
								aria-current={n === currentStep ? 'step' : undefined}
							>
								{#if n < currentStep}<Check size={15} />{:else}{n}{/if}
							</span>
							{#if i < totalSteps - 1}<span class="h-px flex-1 {n < currentStep ? 'bg-[var(--color-primary)]' : 'bg-[var(--color-border)]'}"></span>{/if}
						</div>
						<span class="hidden text-[11px] font-medium sm:block {n === currentStep ? 'text-[var(--color-primary)]' : 'text-[var(--color-muted)]'}">{label}</span>
					</div>
				{/each}
			</nav>
			<!-- Step 1: Welcome & Core Identity -->
			{#if currentStep === 1}
				<div class="mb-6 flex items-center gap-3">
					<Sparkles class="text-[var(--color-primary)]" size={24} />
					<h1 class="text-2xl font-bold text-[var(--color-text)]">{t('onboarding.step1_title', 'Welcome! Tell us about yourself')}</h1>
				</div>
				<div class="space-y-4">
					<div>
						<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="ob-name">{t('onboarding.field_name', 'Your name')}</label>
						<input id="ob-name" bind:value={name} type="text" placeholder={t('onboarding.name_placeholder', 'e.g. Ada Lovelace')} aria-describedby="ob-name-hint" class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none" />
						<HintIcon id="ob-name-hint" text={t('onboarding.name_hint', 'Your public display name — you can change it later.')} />
					</div>
					<div>
						<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="ob-status">{t('onboarding.field_japa_status', 'Where are you in your japa journey?')}</label>
						<AutocompleteSelect
							id="ob-status"
							bind:value={japaStatus}
							options={[
								{ value: '', label: t('onboarding.select_option', 'Select...') },
								...(data.onboardingOptions?.japa_statuses ?? []).map((opt) => ({
									value: opt.value,
									label: t(opt.label_key, opt.value)
								}))
							]}
						/>
						<HintIcon text={t('onboarding.status_hint', 'Your current stage — powers your personalized roadmap.')} />
					</div>
					<div>
						<label class="mb-1 block text-sm font-medium text-[var(--color-text)]">
							{t('onboarding.field_country', 'Country')}
							<Autocomplete
							placeholder={t('onboarding.select_option', 'Select...')}
							emptyMessage={t('common.no_results', 'No results found')}
							fetcher={fetchStep1Countries}
							initialFetcher={fetchStep1CountriesInitial}
							selectedId={selectedCountry || null}
							selectedLabel={selectedCountryLabel}
							onSelect={(item) => (selectedCountry = item.id)}
							onClear={() => (selectedCountry = '')}
						/>
						</label>
							<HintIcon text={t('onboarding.country_hint', 'The country you live in now.')} />
						{#if data.geoContext?.is_valid}
							<p class="mt-1 flex items-center gap-1 text-xs text-[var(--color-muted)]">
								<MapPin size={11} /> {t('onboarding.geo_detected', 'Detected: {country}').replace('{country}', data.geoContext.country_name)}
							</p>
						{/if}
					</div>
					{#if selectedCountry}
						<div>
							<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="ob-state">{t('onboarding.field_state', 'State / Province')}</label>
							{#if loadingStates}
								<p class="text-xs text-[var(--color-muted)]">{t('common.loading', 'Loading...')}</p>
							{:else if states.length > 0}
								<AutocompleteSelect
									id="ob-state"
									bind:value={selectedState}
									options={[
										{ value: '', label: t('onboarding.select_option', 'Select...') },
										...states.map((s) => ({ value: s.id, label: s.name }))
									]}
								/>
								<HintIcon text={t('onboarding.state_hint', 'Your state or province.')} />
							{:else}
								<p class="text-xs text-[var(--color-muted)]">{t('onboarding.no_states', 'No states available for this country')}</p>
							{/if}
						</div>
					{/if}
					{#if selectedState}
						<div>
							<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="ob-city">{t('onboarding.field_city', 'City')}</label>
							{#if loadingCities}
								<p class="text-xs text-[var(--color-muted)]">{t('common.loading', 'Loading...')}</p>
							{:else if cities.length > 0}
								<AutocompleteSelect
									id="ob-city"
									bind:value={selectedCity}
									options={[
										{ value: '', label: t('onboarding.select_option', 'Select...') },
										...cities.map((c) => ({ value: c.id, label: c.name }))
									]}
								/>
								<HintIcon text={t('onboarding.city_hint', 'Pick your city, or the closest one listed.')} />
							{:else}
								<p class="text-xs text-[var(--color-muted)]">{t('onboarding.no_cities', 'No cities available for this state')}</p>
							{/if}
						</div>
					{/if}
					<div>
						<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="ob-location">{t('onboarding.field_location', 'Current location')}</label>
						<input id="ob-location" bind:value={currentLocation} type="text" placeholder={t('onboarding.location_placeholder', 'e.g. Lagos, Nigeria')} aria-describedby="ob-location-hint" class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none" />
						<HintIcon id="ob-location-hint" text={t('onboarding.location_hint', 'Free text if your exact city is not listed above.')} />
					</div>
				</div>
			{/if}

			<!-- Step 2: Destination & Goals -->
			{#if currentStep === 2}
				<h1 class="mb-6 text-2xl font-bold text-[var(--color-text)]">{t('onboarding.step2_title', 'Where are you headed?')}</h1>
				<div class="space-y-4">
					<div>
						<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="ob-dest">{t('onboarding.field_destination', 'Destination country')}</label>
						<Autocomplete
							placeholder={t('onboarding.field_destination_ph', 'Search destination country…')}
							emptyMessage={t('common.no_results', 'No results found')}
							fetcher={destCountryFetcher}
							initialFetcher={destCountryInitialFetcher}
							selectedId={destCountryCode || null}
							selectedLabel={destination || null}
							onSelect={(item) => { destCountryCode = item.id; destination = item.label; }}
							onClear={() => { destCountryCode = ''; destination = ''; }}
						/>
						<HintIcon text={t('onboarding.destination_hint', 'Where you plan to move — drives your tools, guides and feed.')} />
					</div>
					<div>
						<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="ob-goal">{t('onboarding.field_goal', 'Primary goal')}</label>
						<AutocompleteSelect
							id="ob-goal"
							bind:value={primaryGoal}
							options={[
								{ value: '', label: t('onboarding.select_option', 'Select...') },
								...(data.onboardingOptions?.primary_goals ?? []).map((opt) => ({
									value: opt.value,
									label: t(opt.label_key, opt.value)
								}))
							]}
						/>
						<HintIcon text={t('onboarding.goal_hint', 'The main reason for your move — work, study, family, and so on.')} />
					</div>
				</div>
			{/if}

			<!-- Step 3: Interests/Tags -->
			{#if currentStep === 3}
				<h1 class="mb-2 text-2xl font-bold text-[var(--color-text)]">{t('onboarding.step3_title', 'What are you interested in?')}</h1>
				<p class="mb-6 text-sm text-[var(--color-text-subtle)]">{t('onboarding.step3_subtitle', 'Select at least {n} topics to personalize your feed.').replace('{n}', String(minTags))}</p>
				<div class="flex flex-wrap gap-2">
					{#each data.tags as tag (tag.id)}
						<button
							type="button"
							onclick={() => toggleTag(tag.slug)}
							class="rounded-full border px-3 py-1.5 text-sm transition-colors {selectedTags.includes(tag.slug)
								? 'border-[var(--color-primary)] bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
								: 'border-[var(--color-border)] text-[var(--color-text-subtle)] hover:border-[var(--color-primary)]'}"
						>
							#{tag.name}
						</button>
					{/each}
				</div>
				<p class="mt-4 text-xs text-[var(--color-muted)]">{t('onboarding.selected_count', '{n} selected').replace('{n}', String(selectedTags.length))}</p>
			{/if}

			<!-- Step 4: Recommended Users -->
			{#if currentStep === 4}
				<h1 class="mb-2 text-2xl font-bold text-[var(--color-text)]">{t('onboarding.step4_title', 'People to follow')}</h1>
				<p class="mb-6 text-sm text-[var(--color-text-subtle)]">{t('onboarding.step4_subtitle', 'Follow at least {n} people to start your journey.').replace('{n}', String(minUsers))}</p>
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
					{#each userRecs as rec (rec.id)}
						<button
							type="button"
							onclick={() => (selectedUsers = toggleIn(selectedUsers, rec.id))}
							class="flex w-full items-center gap-3 rounded-[var(--radius)] border p-3 text-start transition-colors {selectedUsers.includes(rec.id)
								? 'border-[var(--color-primary)] bg-[var(--color-surface-subtle)]'
								: 'border-[var(--color-border)] hover:bg-[var(--color-surface-subtle)]'}"
						>
							{#if rec.avatar_url}
								<img src={rec.avatar_url} alt="" class="h-10 w-10 rounded-full object-cover" />
							{:else}
								<span class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]">{rec.name.charAt(0)}</span>
							{/if}
							<span class="min-w-0 flex-1">
								<span class="block truncate text-sm font-medium text-[var(--color-text)]">{rec.name}</span>
								<span class="block text-xs text-[var(--color-muted)]">{t('onboarding.reason_' + rec.reason, rec.reason.replace(/_/g, ' '))}</span>
							</span>
							{#if selectedUsers.includes(rec.id)}
								<Check size={18} class="shrink-0 text-[var(--color-primary)]" />
							{/if}
						</button>
					{/each}
				</div>
				<p class="mt-4 text-xs text-[var(--color-muted)]">{t('onboarding.selected_count', '{n} selected').replace('{n}', String(selectedUsers.length))}</p>
			{/if}

			<!-- Step 5: Recommended Spaces -->
			{#if currentStep === 5}
				<h1 class="mb-2 text-2xl font-bold text-[var(--color-text)]">{t('onboarding.step5_title', 'Spaces to join')}</h1>
				<p class="mb-6 text-sm text-[var(--color-text-subtle)]">{t('onboarding.step5_subtitle', 'Join at least {n} spaces to start your journey.').replace('{n}', String(minSpaces))}</p>
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
					{#each spaceRecs as rec (rec.id)}
						<button
							type="button"
							onclick={() => (selectedSpaces = toggleIn(selectedSpaces, rec.id))}
							class="flex w-full items-center gap-3 rounded-[var(--radius)] border p-3 text-start transition-colors {selectedSpaces.includes(rec.id)
								? 'border-[var(--color-primary)] bg-[var(--color-surface-subtle)]'
								: 'border-[var(--color-border)] hover:bg-[var(--color-surface-subtle)]'}"
						>
							{#if rec.avatar_url}
								<img src={rec.avatar_url} alt="" class="h-10 w-10 rounded-full object-cover" />
							{:else}
								<span class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]">{rec.name.charAt(0)}</span>
							{/if}
							<span class="min-w-0 flex-1">
								<span class="block truncate text-sm font-medium text-[var(--color-text)]">{rec.name}</span>
								<span class="block text-xs text-[var(--color-muted)]">{t('onboarding.reason_' + rec.reason, rec.reason.replace(/_/g, ' '))}</span>
							</span>
							{#if selectedSpaces.includes(rec.id)}
								<Check size={18} class="shrink-0 text-[var(--color-primary)]" />
							{/if}
						</button>
					{/each}
				</div>
				<p class="mt-4 text-xs text-[var(--color-muted)]">{t('onboarding.selected_count', '{n} selected').replace('{n}', String(selectedSpaces.length))}</p>
			{/if}

			<!-- Step 6: Sources (optional — no minimum) -->
			{#if currentStep === 6}
				<h1 class="mb-2 text-2xl font-bold text-[var(--color-text)]">{t('onboarding.step6_title', 'Sources to follow')}</h1>
				<p class="mb-6 text-sm text-[var(--color-text-subtle)]">{t('onboarding.step6_subtitle', 'Follow publications and channels to fill your feed. This step is optional.')}</p>
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
					{#each data.sources as source (source.id)}
						<button
							type="button"
							onclick={() => (selectedSources = toggleIn(selectedSources, source.id))}
							class="flex w-full items-center gap-3 rounded-[var(--radius)] border p-3 text-start transition-colors {selectedSources.includes(source.id)
								? 'border-[var(--color-primary)] bg-[var(--color-surface-subtle)]'
								: 'border-[var(--color-border)] hover:bg-[var(--color-surface-subtle)]'}"
						>
							{#if source.logo_url}
								<img src={source.logo_url} alt="" class="h-10 w-10 rounded-full object-cover" />
							{:else}
								<span class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]">{source.name.charAt(0)}</span>
							{/if}
							<span class="min-w-0 flex-1">
								<span class="block truncate text-sm font-medium text-[var(--color-text)]">{source.name}</span>
								<span class="block truncate text-xs text-[var(--color-muted)]">{source.domain}{source.category ? ' · ' + t('sources.category_' + source.category, source.category.replace(/_/g, ' ')) : ''}</span>
							</span>
							{#if selectedSources.includes(source.id)}
								<Check size={18} class="shrink-0 text-[var(--color-primary)]" />
							{/if}
						</button>
					{/each}
				</div>
				<p class="mt-4 text-xs text-[var(--color-muted)]">{t('onboarding.selected_count', '{n} selected').replace('{n}', String(selectedSources.length))}</p>
			{/if}

			<!-- Step 7: First Action Nudge -->
			{#if currentStep === 7}
				<div class="text-center">
					<div class="mx-auto mb-4 grid h-16 w-16 place-items-center rounded-full bg-[var(--color-primary)]">
						<Check size={32} class="text-[var(--color-primary-contrast)]" />
					</div>
					<h1 class="mb-2 text-2xl font-bold text-[var(--color-text)]">{t('onboarding.step7_title', "You're all set!")}</h1>
					<p class="mb-6 text-sm text-[var(--color-text-subtle)]">{t('onboarding.step7_body', 'Your feed is ready. Explore stories, join discussions, and connect with the diaspora community.')}</p>
				</div>

				<!-- §89 Community Achievement Feed — social proof preview.
				     Shows the latest 4 achievements from the community so new
				     registrants see real wins before they land on the main app.
				     Hidden when the feature is disabled or no achievements exist. -->
				{#if data.achievements.length > 0}
					<div class="mt-8 border-t border-[var(--color-border)] pt-6">
						<h2 class="mb-4 flex items-center justify-center gap-2 text-sm font-semibold text-[var(--color-text)]">
							<Trophy size={16} class="text-[var(--color-primary)]" />
							{t('onboarding.achievements_preview_heading', 'Recent wins from the community')}
						</h2>
						<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
							{#each data.achievements as a (a.id)}
								<a
									href={`/achievements/${a.slug}`}
									class="group rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-colors hover:border-[var(--color-primary)] hover:bg-[var(--color-surface-subtle)]"
								>
									<p class="line-clamp-1 text-sm font-medium text-[var(--color-text)]">{a.title}</p>
									<div class="mt-1.5 flex items-center gap-2 text-xs text-[var(--color-text-subtle)]">
										{#if a.milestone_type}
											<span class="inline-flex items-center gap-0.5">
												<Trophy size={11} class="text-[var(--color-primary)]" />
												{milestoneLabel(a.milestone_type)}
											</span>
										{/if}
										{#if a.destination_space_name}
											<span class="inline-flex items-center gap-0.5">
												<MapPin size={11} />
												{a.destination_space_name}
											</span>
										{/if}
										{#if a.reaction_count > 0}
											<span class="inline-flex items-center gap-0.5">
												<Heart size={11} class="text-[var(--color-primary)]" fill="currentColor" />
												{a.reaction_count}
											</span>
										{/if}
									</div>
								</a>
							{/each}
						</div>
						<a
							href="/achievements"
							class="mt-4 block text-center text-xs font-medium text-[var(--color-primary)] hover:underline"
						>
							{t('onboarding.achievements_preview_view_all', 'See more achievements →')}
						</a>
					</div>
				{/if}
			{/if}

			<!-- Error -->
			{#if error}
				<p class="mt-4 text-sm text-red-500">{error}</p>
			{/if}

			<!-- Actions -->
			<div class="mt-8 flex items-center justify-between">
				<div class="flex items-center gap-4">
					{#if currentStep > 1}
						<!-- Back is client-side only — earlier step_data is already
						     persisted, and resubmitting a step merges/overwrites it. -->
						<button
							type="button"
							onclick={() => { if (!saving && currentStep > 1) currentStep--; }}
							disabled={saving}
							class="flex items-center gap-1 text-sm text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)] disabled:opacity-50"
						>
							<ChevronLeft class="rtl:-scale-x-100" size={16} />
							{t('onboarding.back', 'Back')}
						</button>
					{/if}
					{#if currentStep > 2}
						<button
							type="button"
							onclick={skipStep}
							disabled={saving}
							class="text-sm text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)] disabled:opacity-50"
						>
							{t('onboarding.skip', 'Skip')}
						</button>
					{/if}
				</div>
				<button
					type="button"
					onclick={saveStep}
					disabled={saving || !canAdvance}
					class="flex items-center gap-2 rounded-full bg-[var(--color-primary)] px-6 py-2.5 text-sm font-semibold text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
				>
					{saving ? t('onboarding.saving', 'Saving...') : currentStep === totalSteps ? t('onboarding.finish', 'Get started') : t('onboarding.next', 'Continue')}
					{#if currentStep < totalSteps}<ChevronRight class="rtl:-scale-x-100" size={16} />{/if}
				</button>
			</div>

			<!-- Footer: hint + progress dots -->
			<div class="mt-auto flex items-center justify-between gap-4 border-t border-[var(--color-border)] pt-5">
				<p class="flex min-w-0 items-center gap-1.5 text-xs text-[var(--color-muted)]">
					<Lightbulb size={14} class="shrink-0 text-[var(--color-primary)]" />
					<span class="truncate">
						<span class="font-medium">{t('onboarding.why_ask', 'Why we ask this?')}</span>
						{t('onboarding.why_ask_body', 'It helps us show you relevant scholarships, jobs, and opportunities.')}
					</span>
				</p>
				<div class="flex shrink-0 items-center gap-1.5">
					{#each Array(totalSteps) as _, i (i)}
						<span
							class="h-2 w-2 rounded-full transition-colors {i + 1 <= currentStep ? 'bg-[var(--color-primary)]' : 'bg-[var(--color-border)]'}"
						></span>
					{/each}
				</div>
			</div>
		</div>
	</div>
</div>

<style>
	.onb-side {
		background:
			radial-gradient(ellipse 80% 55% at 35% 42%, color-mix(in srgb, var(--color-primary) 12%, transparent), transparent),
			color-mix(in srgb, var(--color-primary) 6%, var(--color-surface));
	}
	.onb-focal {
		position: absolute;
		top: 50%;
		inset-inline-start: 50%;
		translate: -50% -50%;
		display: grid;
		place-items: center;
		width: 8rem;
		height: 8rem;
		border-radius: 9999px;
		background: var(--color-surface);
		box-shadow: 0 10px 28px color-mix(in srgb, var(--color-primary) 15%, transparent);
	}
	.onb-chip {
		position: absolute;
		display: grid;
		place-items: center;
		width: 2.5rem;
		height: 2.5rem;
		border-radius: 0.75rem;
		background: var(--color-surface);
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
	}
	.onb-plane {
		position: absolute;
		top: 10%;
		inset-inline-end: 18%;
		color: var(--color-primary);
		opacity: 0.8;
	}
	@media (prefers-reduced-motion: no-preference) {
		.onb-chip {
			animation: onb-float 4s ease-in-out infinite;
		}
		.onb-chip:nth-of-type(3) { animation-delay: 0.7s; }
		.onb-chip:nth-of-type(4) { animation-delay: 1.4s; }
		.onb-plane {
			animation: onb-float 3.5s ease-in-out infinite;
		}
	}
	@keyframes onb-float {
		0%, 100% { translate: 0 0; }
		50% { translate: 0 -6px; }
	}
</style>

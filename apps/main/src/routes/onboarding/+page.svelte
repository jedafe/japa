<script lang="ts">
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import { Check, ChevronRight, Sparkles, MapPin, Trophy, Heart } from 'lucide-svelte';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import type { PageData } from './$types';
	import type { GeoCity, GeoState, OnboardingRecommendation, TagSuggestion, AchievementMilestoneType } from '$lib/types';

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
	const minRecs = $derived(minSpaces + minUsers);

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

	// Destination country autocomplete — filters the SSR-loaded country
	// spaces client-side (same pattern as LocationSection.svelte).
	function destCountryFetcher(q: string): Promise<AutocompleteItem[]> {
		const needle = q.toLowerCase().trim();
		const list = data.countries ?? [];
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
		const list = data.countries ?? [];
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
		const filtered = query === ''
			? data.countries
			: data.countries.filter(
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

	// Step 4: Recommended Spaces & Connections
	let selectedRecs = $state<string[]>([]);

	// Section 09: per-step gating. Step 1 and 2 are mandatory; Step 3/4 must
	// meet admin-configurable minimums; Step 5 is the terminal finish screen.
	const canAdvance = $derived(
		currentStep === 1
			? name.trim() !== '' && japaStatus !== '' && currentLocation.trim() !== ''
			: currentStep === 2
				? destCountryCode !== '' && primaryGoal !== ''
				: currentStep === 3
					? selectedTags.length >= minTags
					: currentStep === 4
						? selectedRecs.length >= minRecs
						: true
	);

	const totalSteps = 5;
	const stepLabels = $derived([
		t('onboarding.step1', 'Welcome'),
		t('onboarding.step2', 'Destination'),
		t('onboarding.step3', 'Interests'),
		t('onboarding.step4', 'Connect'),
		t('onboarding.step5', 'Get Started')
	]);

	function toggleTag(tagSlug: string) {
		selectedTags = selectedTags.includes(tagSlug)
			? selectedTags.filter((s) => s !== tagSlug)
			: [...selectedTags, tagSlug];
	}

	function toggleRec(id: string) {
		selectedRecs = selectedRecs.includes(id)
			? selectedRecs.filter((s) => s !== id)
			: [...selectedRecs, id];
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
			stepData.follows = selectedRecs;
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
				await goto('/?tour=1');
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
				// Step 5 "skip" is the same as finishing: complete onboarding
				// and redirect to the home feed with the product tour.
				await saveStep();
			}
		} catch {
			error = t('onboarding.error', 'Something went wrong. Please try again.');
		}
		saving = false;
	}

	// §89 Achievement Feed preview — milestone label helper for Step 5.
	function milestoneLabel(m?: string) {
		if (!m) return '';
		return t(`achievements.type_${m}`, m.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase()));
	}
</script>

<svelte:head>
	<title>{t('onboarding.title', 'Welcome to JapaConnect')} — {t('app.name')}</title>
</svelte:head>

<div class="min-h-screen bg-[var(--color-bg)]">
	<div class="mx-auto max-w-full px-6 py-12">
		<!-- Step indicator -->
		<div class="mb-10 flex items-center justify-center gap-2">
			{#each Array(totalSteps) as _, i (i)}
				<div
					class="h-2 rounded-full transition-all {i + 1 === currentStep
						? 'w-8 bg-[var(--color-primary)]'
						: i + 1 < currentStep
							? 'w-2 bg-[var(--color-primary)]'
							: 'w-2 bg-[var(--color-border)]'}"
				></div>
			{/each}
		</div>

		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-8">
			<!-- Step 1: Welcome & Core Identity -->
			{#if currentStep === 1}
				<div class="mb-6 flex items-center gap-3">
					<Sparkles class="text-[var(--color-primary)]" size={24} />
					<h1 class="text-2xl font-bold text-[var(--color-text)]">{t('onboarding.step1_title', 'Welcome! Tell us about yourself')}</h1>
				</div>
				<div class="space-y-4">
					<div>
						<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="ob-name">{t('onboarding.field_name', 'Your name')}</label>
						<input id="ob-name" bind:value={name} type="text" class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none" />
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
							{:else}
								<p class="text-xs text-[var(--color-muted)]">{t('onboarding.no_cities', 'No cities available for this state')}</p>
							{/if}
						</div>
					{/if}
					<div>
						<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="ob-location">{t('onboarding.field_location', 'Current location')}</label>
						<input id="ob-location" bind:value={currentLocation} type="text" placeholder={t('onboarding.location_placeholder', 'e.g. Lagos, Nigeria')} class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none" />
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

			<!-- Step 4: Recommended Spaces & Connections -->
			{#if currentStep === 4}
				<h1 class="mb-2 text-2xl font-bold text-[var(--color-text)]">{t('onboarding.step4_title', 'Connect with your community')}</h1>
				<p class="mb-6 text-sm text-[var(--color-text-subtle)]">{t('onboarding.step4_subtitle', 'Follow at least {n} spaces and people to start your journey.').replace('{n}', String(minRecs))}</p>
				<div class="space-y-2">
					{#each data.recommendations as rec (rec.id)}
						<button
							type="button"
							onclick={() => toggleRec(rec.id)}
							class="flex w-full items-center gap-3 rounded-[var(--radius)] border p-3 text-left transition-colors {selectedRecs.includes(rec.id)
								? 'border-[var(--color-primary)] bg-[var(--color-surface-subtle)]'
								: 'border-[var(--color-border)] hover:bg-[var(--color-surface-subtle)]'}"
						>
							{#if rec.avatar_url}
								<img src={rec.avatar_url} alt="" class="h-10 w-10 rounded-full object-cover" />
							{:else}
								<span class="grid h-10 w-10 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]">{rec.name.charAt(0)}</span>
							{/if}
							<span class="min-w-0 flex-1">
								<span class="block truncate text-sm font-medium text-[var(--color-text)]">{rec.name}</span>
								<span class="block text-xs text-[var(--color-muted)]">{rec.type === 'space' ? t('onboarding.rec_space', 'Space') : t('onboarding.rec_user', 'User')} · {t('onboarding.reason_' + rec.reason, rec.reason.replace(/_/g, ' '))}</span>
							</span>
							{#if selectedRecs.includes(rec.id)}
								<Check size={18} class="text-[var(--color-primary)]" />
							{/if}
						</button>
					{/each}
				</div>
				<p class="mt-4 text-xs text-[var(--color-muted)]">{t('onboarding.selected_count', '{n} selected').replace('{n}', String(selectedRecs.length))}</p>
			{/if}

			<!-- Step 5: First Action Nudge -->
			{#if currentStep === 5}
				<div class="text-center">
					<div class="mx-auto mb-4 grid h-16 w-16 place-items-center rounded-full bg-[var(--color-primary)]">
						<Check size={32} class="text-[var(--color-primary-contrast)]" />
					</div>
					<h1 class="mb-2 text-2xl font-bold text-[var(--color-text)]">{t('onboarding.step5_title', "You're all set!")}</h1>
					<p class="mb-6 text-sm text-[var(--color-text-subtle)]">{t('onboarding.step5_body', 'Your feed is ready. Explore stories, join discussions, and connect with the diaspora community.')}</p>
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
						<div class="grid gap-3 sm:grid-cols-2">
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
				{#if currentStep === 5}
					<button
						type="button"
						onclick={skipStep}
						disabled={saving}
						class="text-sm text-[var(--color-muted)] hover:text-[var(--color-text)] disabled:opacity-50"
					>
						{t('onboarding.skip', 'Skip')}
					</button>
				{:else if currentStep > 2}
					<button
						type="button"
						onclick={skipStep}
						disabled={saving}
						class="text-sm text-[var(--color-muted)] hover:text-[var(--color-text)] disabled:opacity-50"
					>
						{t('onboarding.skip', 'Skip')}
					</button>
				{:else}
					<span></span>
				{/if}
				<button
					type="button"
					onclick={saveStep}
					disabled={saving || !canAdvance}
					class="flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-6 py-2.5 text-sm font-semibold text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
				>
					{saving ? t('onboarding.saving', 'Saving...') : currentStep === totalSteps ? t('onboarding.finish', 'Get started') : t('onboarding.next', 'Next')}
					{#if currentStep < totalSteps}<ChevronRight size={16} />{/if}
				</button>
			</div>
		</div>

		<!-- Step label -->
		<p class="mt-4 text-center text-xs text-[var(--color-muted)]">{currentStep} / {totalSteps} — {stepLabels[currentStep - 1]}</p>
	</div>
</div>

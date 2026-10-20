<script lang="ts">
	/**
	 * Mentor settings — §102 Mentorship Matching (ADR-110) plus the
	 * §114/000829 mentor-card fields and shadow-listing page content.
	 *
	 * Lets elevated-role users (mentorship.become_mentor permission) toggle
	 * their mentor status, set capacity, areas of expertise, and bio.
	 * Calls PATCH /me/mentor-profile. The backend enforces the permission
	 * and validates capacity against the system_settings
	 * `mentorship.max_capacity` cap.
	 *
	 * The 000829 card fields (title, response time, session type, …) and
	 * the structured content arrays populate the mentor's public listing
	 * page via the servicedir shadow listing — the same replace-on-save
	 * contract as the listing editor (empty array clears).
	 *
	 * Ref: japaconnect-engineering/sections/102-mentorship-matching.md
	 *      japaconnect-engineering/sections/114-services-marketplace.md
	 *      japaconnect-engineering/adr/ADR-110-mentorship-matching.md
	 */
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import SettingsSection from '$lib/components/hub/SettingsSection.svelte';
	import ServiceContentEditor, {
		type FactEntry,
		type FaqEntry,
		type IconEntry,
		type StepEntry
	} from '$lib/components/services/ServiceContentEditor.svelte';
	import { api } from '$lib/api/client';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import type { GeoCountry } from '$lib/types';
	import { GraduationCap, Loader2, Check, X, Plus } from 'lucide-svelte';
	import ServiceTierEditor from '$lib/components/services/ServiceTierEditor.svelte';
	import { tiersToPayload, type EditableTier } from '$lib/components/services/serviceForm';
	import type { CurrencyInfo } from '$lib/utils/currency';
	import type { MentorProfile } from '$lib/types';

	let { data }: { data: unknown } = $props();
	const payload = $derived(data as { profile: MentorProfile | null; countries?: GeoCountry[] } | null);
	const profile = $derived(payload?.profile ?? null);
	const geoCountries = $derived(payload?.countries ?? []);

	// §102: mentor settings are gated by the mentorship.become_mentor permission.
	// The backend returns 403 when the user lacks it; the loader catches that
	// and returns null. We also check the feature flag client-side.
	const mentorshipEnabled = $derived(page.data.features?.['mentorship.enabled'] === true);

	// Platform cap for the capacity input, read from the server-provided profile.
	const capacityMax = $derived(profile?.mentor_max_capacity ?? 10);

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] outline-none transition focus:border-[var(--color-primary)]';

	// Form state — seeded from the server-provided profile.
	let isMentor = $state(untrack(() => profile?.is_mentor ?? false));
	let capacity = $state(untrack(() => profile?.mentor_capacity ?? Math.min(5, capacityMax)));
	let areas = $state(untrack(() => (profile?.mentor_areas ?? []).join(', ')));
	let bio = $state(untrack(() => profile?.mentor_bio ?? ''));

	// 000829 mentor-card fields — the public listing's mentor block.
	let mentorTitle = $state(untrack(() => profile?.mentor_title ?? ''));
	let responseTime = $state(untrack(() => profile?.mentor_response_time ?? ''));
	let availability = $state(untrack(() => profile?.mentor_availability ?? ''));
	let sessionType = $state(untrack(() => profile?.mentor_session_type ?? ''));
	let languages = $state(untrack(() => (profile?.mentor_languages ?? []).join(', ')));
	let sessionFormats = $state(untrack(() => (profile?.mentor_session_formats ?? []).join(', ')));
	// Countries is a repeater of geo-backed autocompletes — each row stores a
	// country NAME (the saved mentor_countries contract is string[] of names).
	let countries = $state<string[]>(untrack(() => [...(profile?.mentor_countries ?? [])]));
	function addCountry() {
		countries = [...countries, ''];
	}
	function removeCountry(i: number) {
		countries = countries.filter((_, idx) => idx !== i);
	}
	// Already-picked names are hidden so the same country can't be added twice.
	function countryFetcher(q: string): Promise<AutocompleteItem[]> {
		const needle = q.toLowerCase().trim();
		return Promise.resolve(
			geoCountries
				.filter((c) => !countries.includes(c.name))
				.filter(
					(c) =>
						!needle ||
						c.name.toLowerCase().includes(needle) ||
						c.geo_code.toLowerCase().includes(needle)
				)
				.slice(0, 50)
				.map((c) => ({ id: c.geo_code, label: c.name, sublabel: c.geo_code }))
		);
	}
	function countryInitialFetcher(): Promise<AutocompleteItem[]> {
		return Promise.resolve(
			geoCountries
				.filter((c) => !countries.includes(c.name))
				.slice(0, 8)
				.map((c) => ({ id: c.geo_code, label: c.name, sublabel: c.geo_code }))
		);
	}
	let credentials = $state(untrack(() => (profile?.mentor_credentials ?? []).join(', ')));
	let yearsExperience = $state(untrack(() => profile?.mentor_years_experience ?? 0));
	let successLabel = $state(untrack(() => profile?.mentor_success_label ?? ''));

	// Tabbed layout — one form, one save; switching tabs never submits.
	// The pricing-tiers tab only renders when service_packages.tiers_enabled
	// is on (same gate as the editor block below).
	type MentorTab = 'profile' | 'card' | 'content' | 'tiers';
	let activeTab = $state<MentorTab>('profile');
	const mentorTabs = $derived(
		([
			{ key: 'profile', labelKey: 'mentors.settings_tab_profile', default: 'Profile' },
			{ key: 'card', labelKey: 'mentors.settings_tab_card', default: 'Mentor card' },
			{ key: 'content', labelKey: 'mentors.settings_tab_content', default: 'Content details' },
			{ key: 'tiers', labelKey: 'mentors.settings_tab_tiers', default: 'Pricing tiers' }
		] as const).filter((tab) => tab.key !== 'tiers' || tiersEnabled)
	);
	$effect(() => {
		if (mentorTabs.length > 0 && !mentorTabs.some((tab) => tab.key === activeTab)) {
			activeTab = mentorTabs[0].key;
		}
	});

	// §114/000829 shadow-listing page content — replace-on-save like the
	// listing editor. Empty profile.shadow_content seeds empty arrays.
	let faqs = $state<FaqEntry[]>(
		untrack(() => (profile?.shadow_content?.faqs ?? []).map((f) => ({ ...f })))
	);
	let steps = $state<StepEntry[]>(
		untrack(() =>
			(profile?.shadow_content?.steps ?? []).map((s) => ({
				icon: s.icon ?? '',
				title: s.title,
				description: s.description
			}))
		)
	);
	let sellingPoints = $state<IconEntry[]>(
		untrack(() =>
			(profile?.shadow_content?.selling_points ?? []).map((p) => ({
				icon: p.icon ?? '',
				title: p.title,
				description: p.description ?? ''
			}))
		)
	);
	let facts = $state<FactEntry[]>(
		untrack(() =>
			(profile?.shadow_content?.facts ?? []).map((f) => ({
				icon: f.icon ?? '',
				label: f.label,
				value: f.value
			}))
		)
	);
	let included = $state<IconEntry[]>(
		untrack(() =>
			(profile?.shadow_content?.included ?? []).map((p) => ({
				icon: p.icon ?? '',
				title: p.title,
				description: p.description ?? ''
			}))
		)
	);
	let highlights = $state<string[]>(untrack(() => [...(profile?.shadow_content?.highlights ?? [])]));
	let audience = $state<string[]>(untrack(() => [...(profile?.shadow_content?.audience ?? [])]));

	// §114 2026-10-10 amendment — mentorship "packages" (pricing tiers) on
	// the shadow listing, flag-gated like the listing editors.
	const tiersEnabled = $derived(!!page.data.features?.['service_packages.tiers_enabled']);
	const currenciesList = $derived((page.data.currencies ?? []) as CurrencyInfo[]);
	let tiers = $state<EditableTier[]>(
		untrack(() =>
			(profile?.shadow_content?.tiers ?? []).map((tr) => ({
				name: tr.name,
				badge: tr.badge ?? '',
				description: tr.description ?? '',
				icon: tr.icon ?? '',
				priceAmount: tr.price_amount != null ? String(tr.price_amount / 100) : '',
				currency: tr.price_currency ?? '',
				featuresText: (tr.features ?? []).join('\n'),
				isFeatured: tr.is_featured
			}))
		)
	);

	let saving = $state(false);
	let saved = $state(false);
	let error: string | null = $state(null);

	function csvToArray(value: string): string[] {
		return value
			.split(',')
			.map((a) => a.trim())
			.filter(Boolean);
	}

	// Re-seed when the server payload changes (e.g. after invalidateAll).
	$effect(() => {
		isMentor = profile?.is_mentor ?? false;
		capacity = profile?.mentor_capacity ?? Math.min(5, capacityMax);
		areas = (profile?.mentor_areas ?? []).join(', ');
		bio = profile?.mentor_bio ?? '';
		mentorTitle = profile?.mentor_title ?? '';
		responseTime = profile?.mentor_response_time ?? '';
		availability = profile?.mentor_availability ?? '';
		sessionType = profile?.mentor_session_type ?? '';
		languages = (profile?.mentor_languages ?? []).join(', ');
		sessionFormats = (profile?.mentor_session_formats ?? []).join(', ');
		countries = [...(profile?.mentor_countries ?? [])];
		credentials = (profile?.mentor_credentials ?? []).join(', ');
		yearsExperience = profile?.mentor_years_experience ?? 0;
		successLabel = profile?.mentor_success_label ?? '';
		faqs = (profile?.shadow_content?.faqs ?? []).map((f) => ({ ...f }));
		steps = (profile?.shadow_content?.steps ?? []).map((s) => ({
			icon: s.icon ?? '',
			title: s.title,
			description: s.description
		}));
		sellingPoints = (profile?.shadow_content?.selling_points ?? []).map((p) => ({
			icon: p.icon ?? '',
			title: p.title,
			description: p.description ?? ''
		}));
		facts = (profile?.shadow_content?.facts ?? []).map((f) => ({
			icon: f.icon ?? '',
			label: f.label,
			value: f.value
		}));
		included = (profile?.shadow_content?.included ?? []).map((p) => ({
			icon: p.icon ?? '',
			title: p.title,
			description: p.description ?? ''
		}));
		highlights = [...(profile?.shadow_content?.highlights ?? [])];
		audience = [...(profile?.shadow_content?.audience ?? [])];
		tiers = (profile?.shadow_content?.tiers ?? []).map((tr) => ({
			name: tr.name,
			badge: tr.badge ?? '',
			description: tr.description ?? '',
			icon: tr.icon ?? '',
			priceAmount: tr.price_amount != null ? String(tr.price_amount / 100) : '',
			currency: tr.price_currency ?? '',
			featuresText: (tr.features ?? []).join('\n'),
			isFeatured: tr.is_featured
		}));
	});

	async function save() {
		saving = true;
		saved = false;
		error = null;

		// §114 content validation — mirrors the listing editor so backend
		// codes (invalid_selling_point / invalid_fact / …) don't surprise.
		if (sellingPoints.some((p) => !p.title.trim())) {
			saving = false;
			error = t('services.errors.selling_point_title_required', 'Every selling point needs a title');
			return;
		}
		if (included.some((p) => !p.title.trim())) {
			saving = false;
			error = t('services.errors.included_title_required', 'Every included item needs a title');
			return;
		}
		if (facts.some((f) => !f.label.trim() || !f.value.trim())) {
			saving = false;
			error = t('services.errors.fact_label_value_required', 'Every fact needs a label and a value');
			return;
		}
		if (faqs.some((f) => !f.question.trim())) {
			saving = false;
			error = t('services.errors.faq_question_required', 'Every FAQ needs a question');
			return;
		}
		if (steps.some((s) => !s.title.trim())) {
			saving = false;
			error = t('services.errors.step_title_required', 'Every step needs a title');
			return;
		}
		if (tiersEnabled && tiers.some((tr) => !tr.name.trim())) {
			saving = false;
			error = t('services.errors.tier_name_required', 'Every pricing tier needs a name');
			return;
		}

		const res = await api.patch<MentorProfile>('/me/mentor-profile', {
			is_mentor: isMentor,
			mentor_capacity: Number(capacity),
			mentor_areas: csvToArray(areas),
			mentor_bio: bio.trim() || null,

			// 000829 mentor-card fields — "" clears a stored value.
			mentor_title: mentorTitle.trim(),
			mentor_response_time: responseTime.trim(),
			mentor_availability: availability.trim(),
			mentor_session_type: sessionType.trim(),
			mentor_languages: csvToArray(languages),
			mentor_session_formats: csvToArray(sessionFormats),
			mentor_countries: [...new Set(countries.map((c) => c.trim()).filter(Boolean))],
			mentor_credentials: csvToArray(credentials),
			mentor_years_experience: Math.max(0, Math.floor(Number(yearsExperience) || 0)),
			mentor_success_label: successLabel.trim(),

			// §114/000829 — replace-on-save shadow-listing content: always
			// send every array; an empty array clears the stored set.
			faqs: faqs.map((f) => ({ question: f.question.trim(), answer: f.answer.trim() })),
			steps: steps.map((s) => ({
				icon: s.icon,
				title: s.title.trim(),
				description: s.description.trim()
			})),
			selling_points: sellingPoints.map((p) => ({
				icon: p.icon,
				title: p.title.trim(),
				description: p.description.trim()
			})),
			facts: facts.map((f) => ({ icon: f.icon, label: f.label.trim(), value: f.value.trim() })),
			included: included.map((p) => ({
				icon: p.icon,
				title: p.title.trim(),
				description: p.description.trim()
			})),
			highlights: highlights.map((h) => h.trim()).filter(Boolean),
			audience: audience.map((a) => a.trim()).filter(Boolean),
			// replace-on-save like the other arrays — [] clears the tier set.
			...(tiersEnabled ? { tiers: tiersToPayload(tiers) } : {})
		});
		saving = false;
		if (res.error) {
			error = res.error.message || t('mentors.settings_error', 'Failed to save mentor settings.');
			return;
		}
		saved = true;
		await invalidateAll();
	}
</script>

<SettingsSection
	titleKey="mentors.settings_title"
	descKey="mentors.settings_description"
>
	{#if !mentorshipEnabled}
		<div class="rounded-[var(--radius)] bg-[var(--color-surface-subtle)] p-4 text-sm text-[var(--color-text-subtle)]">
			{t('mentors.settings_disabled', 'Mentorship is not currently available.')}
		</div>
	{:else if !profile}
		<div class="rounded-[var(--radius)] bg-[var(--color-surface-subtle)] p-4 text-sm text-[var(--color-text-subtle)]">
			{t(
				'mentors.settings_no_permission',
				'You do not have permission to become a mentor. Contact an administrator if you believe this is an error.'
			)}
		</div>
	{:else}
		<form onsubmit={(e) => { e.preventDefault(); save(); }} class="space-y-5">
			<div class="mb-1 flex flex-wrap gap-1 border-b border-[var(--color-border)]" role="tablist" aria-label={t('mentors.settings_tabs_label', 'Mentorship settings')}>
				{#each mentorTabs as tab (tab.key)}
					<button
						type="button"
						role="tab"
						aria-selected={activeTab === tab.key}
						onclick={() => (activeTab = tab.key)}
						class="inline-flex items-center gap-1.5 border-b-2 px-3 py-2 text-sm font-medium transition-colors {activeTab === tab.key
							? 'border-[var(--color-primary)] text-[var(--color-primary)]'
							: 'border-transparent text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
					>
						{t(tab.labelKey, tab.default)}
					</button>
				{/each}
			</div>

			{#if activeTab === 'profile'}
			<!-- is_mentor toggle -->
			<div class="flex items-start gap-3">
				<input
					id="is-mentor"
					type="checkbox"
					checked={isMentor}
					onchange={(e) => (isMentor = e.currentTarget.checked)}
					class="mt-1 h-4 w-4 rounded border-[var(--color-border)]"
				/>
				<div>
					<label for="is-mentor" class="text-sm font-medium text-[var(--color-text)]">
						{t('mentors.settings_is_mentor', 'I am available as a mentor')}
					</label>
					<p class="text-xs text-[var(--color-text-subtle)]">
						{t(
							'mentors.settings_is_mentor_help',
							'When enabled, you appear in mentor search results and can receive mentorship requests.'
						)}
					</p>
				</div>
			</div>

			<!-- Capacity -->
			<div class="flex flex-col gap-1">
				<label for="capacity" class="text-sm font-medium text-[var(--color-text)]">
					{t('mentors.settings_capacity', 'Mentee capacity')}
				</label>
				<input
					id="capacity"
					type="number"
					min="1"
					max={capacityMax}
					bind:value={capacity}
					class="w-24 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
				/>
				<p class="text-xs text-[var(--color-text-subtle)]">
					{t(
						'mentors.settings_capacity_help',
						'Maximum number of active mentees. Platform cap: {max}.',
						{ max: capacityMax }
					)}
				</p>
			</div>

			<!-- Areas -->
			<div class="flex flex-col gap-1">
				<label for="areas" class="text-sm font-medium text-[var(--color-text)]">
					{t('mentors.settings_areas', 'Areas of expertise')}
				</label>
				<input
					id="areas"
					type="text"
					bind:value={areas}
					placeholder={t('mentors.settings_areas_placeholder', 'e.g. visa, housing, career')}
					class={inputClass}
				/>
				<p class="text-xs text-[var(--color-text-subtle)]">
					{t('mentors.settings_areas_help', 'Comma-separated keywords used for matching.')}
				</p>
			</div>

			<!-- Bio -->
			<div class="flex flex-col gap-1">
				<label for="bio" class="text-sm font-medium text-[var(--color-text)]">
					{t('mentors.settings_bio', 'Mentor bio')}
				</label>
				<textarea
					id="bio"
					bind:value={bio}
					rows="4"
					maxlength="1000"
					placeholder={t(
						'mentors.settings_bio_placeholder',
						'Share your background and what you can help with.'
					)}
					class="{inputClass} resize-y"
				></textarea>
				<p class="text-xs text-[var(--color-text-subtle)]">
					{bio.length}/1000
				</p>
			</div>

			{/if}

			{#if activeTab === 'card'}
			<!-- §114/000829 mentor card — the headline/stats block on the
				public mentorship listing page. -->
			<fieldset class="space-y-4 rounded-[var(--radius)] border border-[var(--color-border)] p-4">
				<legend class="px-1 text-sm font-semibold text-[var(--color-text)]">
					{t('mentors.settings_card_section', 'Public mentor card')}
				</legend>
				<p class="text-xs text-[var(--color-text-subtle)]">
					{t(
						'mentors.settings_card_help',
						'Shown on your public mentorship listing page. Leave blank to omit.'
					)}
				</p>

				<div class="grid gap-4 sm:grid-cols-2">
					<div class="flex flex-col gap-1">
						<label for="mentor-title" class="text-sm font-medium text-[var(--color-text)]">
							{t('mentors.settings_title_label', 'Title')}
						</label>
						<input
							id="mentor-title"
							type="text"
							bind:value={mentorTitle}
							placeholder={t('mentors.settings_title_ph', 'e.g. Immigration Consultant')}
							class={inputClass}
						/>
					</div>
					<div class="flex flex-col gap-1">
						<label for="mentor-response" class="text-sm font-medium text-[var(--color-text)]">
							{t('mentors.settings_response_time', 'Response time')}
						</label>
						<input
							id="mentor-response"
							type="text"
							bind:value={responseTime}
							placeholder={t('mentors.settings_response_time_ph', 'e.g. Usually replies within 24h')}
							class={inputClass}
						/>
					</div>
					<div class="flex flex-col gap-1">
						<label for="mentor-availability" class="text-sm font-medium text-[var(--color-text)]">
							{t('mentors.settings_availability', 'Availability')}
						</label>
						<input
							id="mentor-availability"
							type="text"
							bind:value={availability}
							placeholder={t('mentors.settings_availability_ph', 'e.g. Weekdays, evenings')}
							class={inputClass}
						/>
					</div>
					<div class="flex flex-col gap-1">
						<label for="mentor-session-type" class="text-sm font-medium text-[var(--color-text)]">
							{t('mentors.settings_session_type', 'Session type')}
						</label>
						<input
							id="mentor-session-type"
							type="text"
							bind:value={sessionType}
							placeholder={t('mentors.settings_session_type_ph', 'e.g. 1:1 video calls')}
							class={inputClass}
						/>
					</div>
					<div class="flex flex-col gap-1">
						<label for="mentor-years" class="text-sm font-medium text-[var(--color-text)]">
							{t('mentors.settings_years', 'Years of experience')}
						</label>
						<input
							id="mentor-years"
							type="number"
							min="0"
							max="150"
							bind:value={yearsExperience}
							class={inputClass}
						/>
					</div>
					<div class="flex flex-col gap-1">
						<label for="mentor-success" class="text-sm font-medium text-[var(--color-text)]">
							{t('mentors.settings_success_label', 'Success label')}
						</label>
						<input
							id="mentor-success"
							type="text"
							bind:value={successLabel}
							placeholder={t('mentors.settings_success_label_ph', 'e.g. 95% visa approval rate')}
							class={inputClass}
						/>
					</div>
				</div>

				<div class="grid gap-4 sm:grid-cols-2">
					<div class="flex flex-col gap-1">
						<label for="mentor-languages" class="text-sm font-medium text-[var(--color-text)]">
							{t('mentors.settings_languages', 'Languages')}
						</label>
						<input
							id="mentor-languages"
							type="text"
							bind:value={languages}
							placeholder={t('mentors.settings_languages_ph', 'e.g. English, French')}
							class={inputClass}
						/>
					</div>
					<div class="flex flex-col gap-1">
						<label for="mentor-formats" class="text-sm font-medium text-[var(--color-text)]">
							{t('mentors.settings_formats', 'Session formats')}
						</label>
						<input
							id="mentor-formats"
							type="text"
							bind:value={sessionFormats}
							placeholder={t('mentors.settings_formats_ph', 'e.g. Video call, chat')}
							class={inputClass}
						/>
					</div>
					<div class="flex flex-col gap-1">
						<span class="text-sm font-medium text-[var(--color-text)]">
							{t('mentors.settings_countries', 'Countries')}
						</span>
						{#each countries as country, i (i)}
							<div class="flex items-center gap-2">
								<div class="min-w-0 flex-1">
									<Autocomplete
										placeholder={t('mentors.settings_countries_ph', 'Search country…')}
										emptyMessage={t('settings.location.no_countries', 'No countries found')}
										fetcher={countryFetcher}
										initialFetcher={countryInitialFetcher}
										selectedId={country || null}
										selectedLabel={country || null}
										onSelect={(item) => (countries[i] = item.label)}
										onClear={() => removeCountry(i)}
									/>
								</div>
								<button
									type="button"
									onclick={() => removeCountry(i)}
									aria-label={t('mentors.settings_remove_country', 'Remove country')}
									title={t('mentors.settings_remove_country', 'Remove country')}
									class="grid h-8 w-8 shrink-0 place-items-center rounded-[var(--radius)] border border-[var(--color-border)] text-[var(--color-muted)] transition-colors hover:border-[var(--color-danger)] hover:text-[var(--color-danger)]"
								>
									<X size={14} />
								</button>
							</div>
						{/each}
						<button
							type="button"
							onclick={addCountry}
							class="mt-1 inline-flex w-fit items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							<Plus size={14} /> {t('mentors.settings_add_country', 'Add country')}
						</button>
					</div>
					<div class="flex flex-col gap-1">
						<label for="mentor-credentials" class="text-sm font-medium text-[var(--color-text)]">
							{t('mentors.settings_credentials', 'Credentials')}
						</label>
						<input
							id="mentor-credentials"
							type="text"
							bind:value={credentials}
							placeholder={t('mentors.settings_credentials_ph', 'e.g. RCIC, IELTS coach')}
							class={inputClass}
						/>
					</div>
				</div>
				<p class="text-xs text-[var(--color-text-subtle)]">
					{t('mentors.settings_csv_help', 'Comma-separated values.')}
				</p>
			</fieldset>
			{/if}

			{#if activeTab === 'content'}
			<!-- §114/000829 shadow-listing page content — same editor as the
				listing create/edit forms, shared via ServiceContentEditor. -->
			<ServiceContentEditor
				bare
				{inputClass}
				bind:faqs
				bind:steps
				bind:sellingPoints
				bind:facts
				bind:included
				bind:highlights
				bind:audience
			/>

			{/if}

			{#if activeTab === 'tiers'}
			<!-- §114 2026-10-10 amendment — mentorship pricing tiers render as
				the "Mentorship Packages" cards on the shadow listing. -->
			{#if tiersEnabled}
				<div class="rounded-[var(--radius)] border border-[var(--color-border)] p-4">
					<ServiceTierEditor bind:tiers {inputClass} currencies={currenciesList} />
				</div>
			{/if}
			{/if}

			{#if error}
				<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
					{error}
				</p>
			{/if}
			{#if saved}
				<p class="inline-flex items-center gap-1.5 text-sm text-green-600">
					<Check size={14} /> {t('mentors.settings_saved', 'Saved.')}
				</p>
			{/if}

			<div class="flex justify-end">
				<button
					type="submit"
					disabled={saving}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
				>
					{#if saving}
						<Loader2 size={14} class="animate-spin" />
					{:else}
						<GraduationCap size={14} />
					{/if}
					{t('common.save', 'Save')}
				</button>
			</div>
		</form>
	{/if}
</SettingsSection>

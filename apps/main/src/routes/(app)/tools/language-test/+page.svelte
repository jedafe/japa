<script lang="ts">
	import { untrack } from 'svelte';
	import { t, i18n } from '$lib/i18n';
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import { enhance } from '$app/forms';
	import {
		Languages,
		Lock,
		ArrowLeft,
		ExternalLink,
		Star,
		Plus,
		Clock,
		MapPin,
		MessageSquare,
		ChevronRight,
		BookOpen,
		GitCompare,
		Loader2
	} from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import AiBodyField from '$lib/ai/AiBodyField.svelte';
	import type {
		LanguageTest,
		LanguageTestRequirementWithAi,
		LanguageTestResource,
		LanguageTestScoreStory,
		GeoCountry
	} from '$lib/types';
	import type { PageData, ActionData } from './$types';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';

	/**
	 * Language Test Hub tool — §99, ADR-088.
	 * Two modes:
	 * 1. Requirements mode (default): Destination selector → optional visa
	 *    category → list of requirements. Each requirement links to test detail.
	 * 2. Test detail mode (when a test is selected): test info, resources list,
	 *    score stories, and a story submission form for authed users.
	 */
	let { data, form }: { data: PageData; form?: ActionData } = $props();

	const title = $derived(t('language_test.title', 'Language Test Hub'));
	const description = $derived(
		t(
			'language_test.subtitle',
			'Find required language tests for your visa, minimum scores, prep resources, and community score stories.'
		)
	);

	const disabled = $derived(data.disabled);
	const authed = $derived(data.authed);

	let destinations = $state<GeoCountry[]>(untrack(() => data.destinations));
	let tests = $state<LanguageTest[]>(untrack(() => data.tests));
	let allRequirements = $state<LanguageTestRequirementWithAi[]>(untrack(() => data.requirements));

	let selectedDestId = $state<string>(untrack(() => data.initialDestId ?? ''));
	let selectedVisaCategory = $state<string>('');
	let loadingRequirements = $state(false);
	// Race guard — stale responses must not overwrite a newer destination's data.
	let requirementsToken = 0;

	// Test detail mode
	let selectedTestSlug = $state<string | null>(null);
	let testDetail = $state<LanguageTest | null>(null);
	let testResources = $state<LanguageTestResource[]>([]);
	let testStories = $state<LanguageTestScoreStory[]>([]);
	let loadingDetail = $state(false);
	let detailError = $state<string | null>(null);
	// Race guard — clicking two tests quickly must not let the slower
	// response overwrite the newer test's detail.
	let detailToken = 0;

	// Story submission form
	let showRegisterPrompt = $state(false);
	let showStoryForm = $state(false);
	let storyFinalScore = $state<string>('');
	let storyStartingLevel = $state<string>('');
	let storyPrepWeeks = $state<number | null>(null);
	let storyResourcesUsed = $state<string>('');
	let storyAdvice = $state<string>('');
	let submittingStory = $state(false);
	let storyError = $state<string | null>(null);

	// JCE comparison selection
	let selectedTestSlugs = $state<Set<string>>(new Set());
	let savingJCE = $state(false);

	const canCompare = $derived(selectedTestSlugs.size >= 2 && selectedTestSlugs.size <= 4);

	// Visa categories available for the selected destination, derived from the
	// loaded requirements (the API matches visa_category exactly — suggestions
	// must come from real data, not free text).
	const visaCategories = $derived(
		Array.from(
			new Set(
				allRequirements
					.map((r) => r.visa_category)
					.filter((c): c is string => !!c)
			)
		).sort()
	);

	// Requirements shown in the list — the ?visa= filter is an exact match on
	// the backend, so filtering client-side on the already-fetched set is
	// equivalent and avoids a request per selection change.
	const requirements = $derived(
		selectedVisaCategory
			? allRequirements.filter((r) => r.visa_category === selectedVisaCategory)
			: allRequirements
	);

	async function loadRequirements() {
		const destId = selectedDestId;
		const token = ++requirementsToken;
		if (!destId) {
			allRequirements = [];
			loadingRequirements = false;
			return;
		}
		loadingRequirements = true;
		const res = await api.get<LanguageTestRequirementWithAi[]>(
			`/language-tests/requirements?dest=${encodeURIComponent(destId)}`
		);
		if (token !== requirementsToken) return;
		loadingRequirements = false;
		allRequirements = res.error ? [] : (res.data ?? []);
	}

	function onDestChange(id: string) {
		selectedDestId = id;
		selectedVisaCategory = '';
		selectedTestSlug = null;
		selectedTestSlugs = new Set();
		loadRequirements();
		const url = new URL(page.url);
		if (id) url.searchParams.set('dest', id);
		else url.searchParams.delete('dest');
		replaceState(url, {});
	}

	function toggleTestSelection(slug: string) {
		const next = new Set(selectedTestSlugs);
		if (next.has(slug)) {
			next.delete(slug);
		} else {
			if (next.size >= 4) return;
			next.add(slug);
		}
		selectedTestSlugs = next;
	}

	/** Enter test detail mode — fetch test info, resources, and stories. */
	async function selectTest(slug: string) {
		selectedTestSlug = slug;
		const token = ++detailToken;
		loadingDetail = true;
		detailError = null;
		testDetail = null;
		testResources = [];
		testStories = [];
		showStoryForm = false;
		storyError = null;

		// Fetch test detail (nested data object: { test, resources_summary, stories_summary })
		const detailRes = await api.get<{
			test: LanguageTest;
			resources_summary: LanguageTestResource[];
			stories_summary: LanguageTestScoreStory[];
		}>(`/language-tests/${encodeURIComponent(slug)}`);
		if (token !== detailToken) return;
		if (detailRes.error || !detailRes.data) {
			detailError = detailRes.error?.message ?? t('language_test.detail_error', 'Could not load this test.');
			loadingDetail = false;
			return;
		}
		testDetail = detailRes.data.test;
		testResources = detailRes.data.resources_summary ?? [];
		testStories = detailRes.data.stories_summary ?? [];

		// Fetch the full resources list and stories (optionally filtered by
		// destination) in parallel.
		const storyParams = new URLSearchParams();
		if (selectedDestId) storyParams.set('dest', selectedDestId);
		const [resRes, storyRes] = await Promise.all([
			api.get<LanguageTestResource[]>(`/language-tests/${encodeURIComponent(slug)}/resources`),
			api.get<LanguageTestScoreStory[]>(
				`/language-tests/${encodeURIComponent(slug)}/stories?${storyParams.toString()}`
			)
		]);
		if (token !== detailToken) return;
		if (!resRes.error && resRes.data) {
			testResources = resRes.data;
		}
		if (!storyRes.error && storyRes.data) {
			testStories = storyRes.data;
		}

		loadingDetail = false;
	}

	function backToRequirements() {
		selectedTestSlug = null;
		testDetail = null;
		testResources = [];
		testStories = [];
		detailError = null;
		showStoryForm = false;
	}

	function openStoryForm() {
		if (!authed) {
			showRegisterPrompt = true;
			return;
		}
		storyError = null;
		showStoryForm = true;
	}

	function closeStoryForm() {
		showStoryForm = false;
		storyError = null;
	}

	async function submitStory() {
		if (!selectedTestSlug) return;
		submittingStory = true;
		storyError = null;
		const body: Record<string, unknown> = {
			final_score: storyFinalScore.trim()
		};
		if (storyStartingLevel) body.starting_level = storyStartingLevel;
		if (selectedDestId) body.destination_space_id = selectedDestId;
		if (storyPrepWeeks && storyPrepWeeks > 0) body.prep_duration_weeks = storyPrepWeeks;
		if (storyResourcesUsed) {
			body.resources_used = storyResourcesUsed
				.split(',')
				.map((s) => s.trim())
				.filter(Boolean);
		}
		if (storyAdvice) body.advice = storyAdvice;
		const res = await api.post<LanguageTestScoreStory>(
			`/language-tests/${encodeURIComponent(selectedTestSlug)}/stories`,
			body
		);
		submittingStory = false;
		if (res.error) {
			storyError = res.error.message;
			return;
		}
		if (res.data) {
			testStories = [res.data, ...testStories];
		}
		storyFinalScore = '';
		storyStartingLevel = '';
		storyPrepWeeks = null;
		storyResourcesUsed = '';
		storyAdvice = '';
		showStoryForm = false;
	}

	/** Get score_breakdown entries from a JSON object field. */
	function breakdownEntries(
		val: Record<string, unknown> | null | undefined
	): [string, unknown][] {
		if (!val || typeof val !== 'object') return [];
		return Object.entries(val);
	}

	/** Render a score_breakdown cell — nested values come through as JSON. */
	function breakdownValue(v: unknown): string {
		if (v == null) return '—';
		if (typeof v === 'object') return JSON.stringify(v);
		return String(v);
	}

	/** Format an ISO date string as a readable date. */
	function formatDate(iso?: string | null): string {
		if (!iso) return '';
		const d = new Date(iso);
		if (isNaN(d.getTime())) return iso;
		return d.toLocaleDateString(i18n.locale, {
			year: 'numeric',
			month: 'short',
			day: 'numeric'
		});
	}

	// Autocomplete fetchers — filter the already-loaded destinations client-side.
	function fetchDestinations(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		const filtered = query === ''
			? destinations
			: destinations.filter((d) => d.name.toLowerCase().includes(query) || (d.geo_code ?? '').toLowerCase().includes(query));
		return Promise.resolve(filtered.map((d) => ({ id: d.id, label: d.name, sublabel: d.geo_code })));
	}
	function fetchDestinationsInitial(): Promise<AutocompleteItem[]> {
		return fetchDestinations('');
	}

	// Visa category fetchers — suggestions come from the loaded requirements
	// (exact-match filter, so only real categories are meaningful).
	function fetchVisaCategories(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		const filtered =
			query === ''
				? visaCategories
				: visaCategories.filter((c) => c.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((c) => ({ id: c, label: c })));
	}
	function fetchVisaCategoriesInitial(): Promise<AutocompleteItem[]> {
		return fetchVisaCategories('');
	}

	const selectedDestLabel = $derived(
		selectedDestId ? destinations.find((d) => d.id === selectedDestId)?.name ?? null : null
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<link rel="canonical" href={page.url.href} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:url" content={page.url.href} />
	<meta property="og:type" content="website" />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Languages size={20} /> {title}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">{description}</p>
	</div>

	{#if disabled}
		<div class="p-4">
			<EmptyState
				icon={Lock}
				title={t('tools.disabled_title', 'This tool is currently unavailable')}
				body={t('tools.disabled_body', 'Check back soon or contact support.')}
			/>
		</div>
	{:else if destinations.length === 0}
		<div class="p-4">
			<EmptyState
				icon={Languages}
				title={t('language_test.no_destinations', 'No destinations available')}
				body={t(
					'language_test.no_destinations_desc',
					"Language test data hasn't been configured yet. Please check back later."
				)}
			/>
		</div>
	{:else if selectedTestSlug}
		<!-- ===== Test detail mode ===== -->
		<div class="p-4 space-y-6">
			<button
				type="button"
				onclick={backToRequirements}
				class="inline-flex items-center gap-1.5 text-sm text-[var(--color-muted)] hover:text-[var(--color-text)] transition-colors"
			>
				<ArrowLeft class="rtl:-scale-x-100" size={16} />
				{t('language_test.back_to_requirements', 'Back to requirements')}
			</button>

			{#if loadingDetail}
				<div class="space-y-3">
					<LoadingSkeleton variant="card" count={1} />
					<LoadingSkeleton variant="lines" count={4} />
					<LoadingSkeleton variant="lines" count={4} />
				</div>
			{:else if detailError}
				<EmptyState
					icon={Languages}
					title={t('language_test.detail_error_title', 'Could not load test')}
					body={detailError}
				/>
			{:else if testDetail}
				<!-- Test info -->
				<div class="p-4 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius-card)]">
					<div class="flex items-center gap-2 flex-wrap">
						<h2 class="text-xl font-semibold text-[var(--color-text)]">{testDetail.name}</h2>
						{#if testDetail.validity_months}
							<span class="text-xs px-2 py-0.5 bg-[var(--color-surface-subtle)] text-[var(--color-primary)] rounded-full font-medium">
								{t('language_test.valid_for', 'Valid for')} {testDetail.validity_months} {t('language_test.months', 'months')}
							</span>
						{/if}
					</div>
					{#if testDetail.official_url}
						<a
							href={testDetail.official_url}
							target="_blank"
							rel="noopener noreferrer"
							class="mt-2 inline-flex items-center gap-1.5 text-sm text-[var(--color-primary)] hover:underline"
						>
							<ExternalLink class="rtl:-scale-x-100" size={14} />
							{t('language_test.official_website', 'Official website')}
						</a>
					{/if}
				</div>

				<!-- Resources -->
				<div>
					<h3 class="text-sm font-semibold text-[var(--color-muted)] uppercase tracking-wide mb-3">
						{t('language_test.resources', 'Prep resources')}
					</h3>
					{#if testResources.length === 0}
						<EmptyState
							icon={BookOpen}
							title={t('language_test.no_resources', 'No resources')}
							body={t(
								'language_test.no_resources_desc',
								'No prep resources have been added for this test yet.'
							)}
						/>
					{:else}
						<div class="space-y-2">
							{#each testResources as resource (resource.id)}
								<div class="p-3 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius-card)] flex items-center justify-between gap-3">
									<div class="min-w-0">
										<a
											href={resource.resource_url}
											target="_blank"
											rel="noopener noreferrer"
											class="text-sm font-medium text-[var(--color-text)] hover:text-[var(--color-primary)] transition-colors"
										>
											{resource.title}
										</a>
										<div class="flex items-center gap-2 text-xs text-[var(--color-muted)] mt-0.5">
											{#if resource.resource_type}
												<span>{resource.resource_type}</span>
											{/if}
											{#if resource.community_rating}
												<span class="flex items-center gap-0.5">
													<Star size={12} class="text-yellow-500" />
													{resource.community_rating}
													{#if resource.rating_count > 0}
														({resource.rating_count})
													{/if}
												</span>
											{/if}
										</div>
									</div>
									<a
										href={resource.resource_url}
										target="_blank"
										rel="noopener noreferrer"
										class="text-[var(--color-muted)] hover:text-[var(--color-text)] flex-shrink-0"
										aria-label={t('language_test.open_resource', 'Open resource')}
									>
										<ExternalLink class="rtl:-scale-x-100" size={16} />
									</a>
								</div>
							{/each}
						</div>
					{/if}
				</div>

				<!-- Score stories -->
				<div>
					<div class="flex items-center justify-between mb-3">
						<h3 class="text-sm font-semibold text-[var(--color-muted)] uppercase tracking-wide">
							{t('language_test.score_stories', 'Community score stories')}
						</h3>
						<button
							type="button"
							onclick={openStoryForm}
							class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium text-white bg-[var(--color-primary)] rounded-[var(--radius)] hover:bg-[var(--color-primary-hover)] transition-colors"
						>
							<Plus size={16} />
							{t('language_test.share_story', 'Share your story')}
						</button>
					</div>

					<!-- Story submission form -->
					{#if authed && showStoryForm}
						<div class="mb-4 p-4 bg-[var(--color-surface-subtle)] border border-[var(--color-border)] rounded-[var(--radius-card)] space-y-3">
							{#if storyError}
								<p class="text-sm text-[var(--color-danger)]">{storyError}</p>
							{/if}
							<div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
								<div>
									<label for="story-score" class="block text-xs font-medium text-[var(--color-muted)] mb-1">
										{t('language_test.final_score', 'Final score')} *
									</label>
									<p class="mt-1 text-xs text-[var(--color-muted)]">{t('tools.lang_test.score_hint', 'Your test score — e.g. IELTS 7.5.')}</p>
									<input
										id="story-score"
										type="text"
										bind:value={storyFinalScore}
										placeholder={t('language_test.score_placeholder', 'e.g. 7.5')}
										class="w-full px-3 py-2 text-sm bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] focus:border-[var(--color-primary)] focus:outline-none"
									/>
								</div>
								<div>
									<label for="story-level" class="block text-xs font-medium text-[var(--color-muted)] mb-1">
										{t('language_test.starting_level', 'Starting level')}
									</label>
									<p class="mt-1 text-xs text-[var(--color-muted)]">{t('tools.lang_test.level_hint', 'Your level — e.g. B1, CLB 8.')}</p>
									<input
										id="story-level"
										type="text"
										bind:value={storyStartingLevel}
										placeholder={t('language_test.level_placeholder', 'e.g. B1')}
										class="w-full px-3 py-2 text-sm bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] focus:border-[var(--color-primary)] focus:outline-none"
									/>
								</div>
								<div>
									<label for="story-weeks" class="block text-xs font-medium text-[var(--color-muted)] mb-1">
										{t('language_test.prep_weeks', 'Prep duration (weeks)')}
									</label>
									<p class="mt-1 text-xs text-[var(--color-muted)]">{t('tools.lang_test.weeks_hint', 'How many weeks you prepared.')}</p>
									<input
										id="story-weeks"
										type="number"
										bind:value={storyPrepWeeks}
										min="0"
										class="w-full px-3 py-2 text-sm bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] focus:border-[var(--color-primary)] focus:outline-none"
									/>
								</div>
								<div>
									<label for="story-resources" class="block text-xs font-medium text-[var(--color-muted)] mb-1">
										{t('language_test.resources_used', 'Resources used (comma-separated)')}
									</label>
									<p class="mt-1 text-xs text-[var(--color-muted)]">{t('tools.lang_test.resources_hint', 'What you studied with — courses, books, channels.')}</p>
									<input
										id="story-resources"
										type="text"
										bind:value={storyResourcesUsed}
										placeholder={t('language_test.resources_placeholder', 'e.g. IELTS Liz, YouTube, Cambridge books')}
										class="w-full px-3 py-2 text-sm bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] focus:border-[var(--color-primary)] focus:outline-none"
									/>
								</div>
							</div>
							<div>
								<AiBodyField
									id="story-advice"
									label={t('language_test.advice', 'Advice')}
									contentType="language_test"
									title={testDetail.name}
									body={storyAdvice}
									onBody={(v) => (storyAdvice = v)}
									placeholder={t('language_test.advice_placeholder', 'Share your prep advice for others...')}
									rows={3}
									user={data.user}
									features={data.features ?? {}}
								/>
							</div>
							<div class="flex items-center gap-3">
								<button
									type="button"
									onclick={submitStory}
									disabled={submittingStory || !storyFinalScore.trim()}
									class="px-4 py-2 text-sm font-medium text-white bg-[var(--color-primary)] rounded-[var(--radius)] hover:bg-[var(--color-primary-hover)] transition-colors disabled:opacity-50"
								>
									{submittingStory
										? t('common.submitting', 'Submitting...')
										: t('common.submit', 'Submit')}
								</button>
								<button
									type="button"
									onclick={closeStoryForm}
									class="px-4 py-2 text-sm font-medium text-[var(--color-muted)] hover:text-[var(--color-text)] transition-colors"
								>
									{t('common.cancel', 'Cancel')}
								</button>
							</div>
						</div>
					{/if}

					{#if testStories.length === 0}
						<EmptyState
							icon={MessageSquare}
							title={t('language_test.no_stories', 'No score stories')}
							body={t(
								'language_test.no_stories_desc',
								'No community score stories have been shared for this test yet.'
							)}
						/>
					{:else}
						<div class="space-y-2">
							{#each testStories as story (story.id)}
								<div class="p-4 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius-card)]">
									<div class="flex items-center gap-2 flex-wrap mb-2">
										{#if story.author_name}
											<span class="text-sm font-medium text-[var(--color-text)]">{story.author_name}</span>
										{/if}
										<span class="text-xs px-2 py-0.5 bg-[var(--color-surface-subtle)] text-[var(--color-primary)] rounded-full font-semibold">
											{t('language_test.score', 'Score:')} {story.final_score}
										</span>
										{#if story.starting_level}
											<span class="text-xs text-[var(--color-muted)]">
												{t('language_test.from', 'from')} {story.starting_level}
											</span>
										{/if}
									</div>
									<div class="flex items-center gap-3 text-xs text-[var(--color-muted)] mb-2">
										{#if story.prep_duration_weeks}
											<span class="flex items-center gap-1">
												<Clock size={12} />
												{story.prep_duration_weeks} {t('language_test.weeks', 'weeks')}
											</span>
										{/if}
										{#if story.destination_space_name}
											<span class="flex items-center gap-1">
												<MapPin size={12} />
												{story.destination_space_name}
											</span>
										{/if}
										<span>{formatDate(story.created_at)}</span>
									</div>
									{#if story.resources_used && story.resources_used.length > 0}
										<div class="flex items-center gap-1.5 flex-wrap mb-2">
											<span class="text-xs text-[var(--color-muted)]">{t('language_test.resources', 'Resources:')}</span>
											{#each story.resources_used as res, i (i)}
												<span class="text-xs px-1.5 py-0.5 bg-[var(--color-surface-subtle)] rounded">{res}</span>
											{/each}
										</div>
									{/if}
									{#if story.advice}
										<p class="text-sm text-[var(--color-text-subtle)]">{story.advice}</p>
									{/if}
								</div>
							{/each}
						</div>
					{/if}
				</div>
			{/if}
		</div>
	{:else}
		<!-- ===== Requirements mode (default) ===== -->
		<!-- Destination selector (geo-aware pattern — first input) -->
		<div class="border-b border-[var(--color-border)] px-4 py-3">
			<label for="lt-dest" class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
				{t('language_test.destination', 'Destination country')}
			</label>
			<div class="mt-2">
				<Autocomplete
					id="lt-dest"
					placeholder={t('language_test.select_destination', 'Select a destination country')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={fetchDestinations}
					initialFetcher={fetchDestinationsInitial}
					selectedId={selectedDestId || null}
					selectedLabel={selectedDestLabel}
					onSelect={(item) => onDestChange(item.id)}
					onClear={() => onDestChange('')}
				/>
			</div>
		</div>

		{#if selectedDestId && visaCategories.length > 0}
			<!-- Optional visa category filter -->
			<div class="border-b border-[var(--color-border)] px-4 py-3 flex items-center gap-3">
				<label for="visa-cat" class="text-sm font-medium text-[var(--color-text)] whitespace-nowrap">
					{t('language_test.visa_category', 'Visa category:')}
				</label>
				<div class="flex-1">
					<Autocomplete
						id="visa-cat"
						placeholder={t('language_test.visa_placeholder', 'Filter by visa category (optional)')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchVisaCategories}
						initialFetcher={fetchVisaCategoriesInitial}
						selectedId={selectedVisaCategory || null}
						selectedLabel={selectedVisaCategory || null}
						onSelect={(item) => (selectedVisaCategory = item.id)}
						onClear={() => (selectedVisaCategory = '')}
						compact
					/>
				</div>
			</div>
		{/if}

		{#if loadingRequirements}
			<div class="p-4 space-y-3">
				<LoadingSkeleton variant="card" count={3} />
			</div>
		{:else if !selectedDestId}
			<div class="p-4">
				<EmptyState
					icon={MapPin}
					title={t('language_test.select_destination', 'Select a destination')}
					body={t(
						'language_test.select_destination_desc',
						'Choose a country to see language test requirements for that destination.'
					)}
				/>
			</div>
		{:else if requirements.length === 0}
			<div class="p-4">
				<EmptyState
					icon={Languages}
					title={t('language_test.no_requirements', 'No requirements')}
					body={selectedVisaCategory
						? t(
								'language_test.no_requirements_filtered',
								'No language test requirements for this visa category. Try a different category.'
							)
						: t(
								'language_test.no_requirements_desc',
								'No language test requirements have been published for this destination yet.'
							)}
				/>
			</div>
		{:else}
			<div class="p-4 space-y-3">
				<div class="mb-4 flex items-center justify-between flex-wrap gap-3">
					<p class="text-sm text-[var(--color-muted)]">
						{t('language_test.selected', 'Selected')}: {selectedTestSlugs.size} / 4
					</p>
					<form
						method="POST"
						use:enhance={() => {
							savingJCE = true;
							return async ({ update }) => {
								savingJCE = false;
								await update();
							};
						}}
						class="contents"
					>
						<input type="hidden" name="dest_id" value={selectedDestId} />
						{#each Array.from(selectedTestSlugs) as slug (slug)}
							<input type="hidden" name="test_slug" value={slug} />
						{/each}
						<button
							type="submit"
							disabled={!canCompare}
							onclick={(e) => {
								if (!authed) {
									e.preventDefault();
									showRegisterPrompt = true;
								}
							}}
							title={authed
								? t('language_test.compare_in_jce_hint', 'Compare in JCE')
								: t('language_test.sign_in_to_compare', 'Sign in to compare')}
							class="inline-flex items-center gap-2 px-4 py-2 bg-[var(--color-primary)] text-[var(--color-primary-contrast)] rounded-[var(--radius)] text-sm font-medium hover:bg-[var(--color-primary-hover)] transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
						>
							{#if savingJCE}
								<Loader2 size={16} class="animate-spin" />
							{:else}
								<GitCompare size={16} />
							{/if}
							{t('language_test.compare_in_jce', 'Compare in JCE')}
						</button>
					</form>
				</div>

				{#if form?.error}
					<div class="mb-4 rounded-[var(--radius)] border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300">
						{form.message || t('language_test.jce_error', 'Could not create comparison')}
					</div>
				{/if}

				{#each requirements as req (req.id)}
					<div class="p-4 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius-card)] {selectedTestSlugs.has(req.test_slug) ? 'border-[var(--color-primary)] ring-1 ring-[var(--color-primary)]' : ''}">
						<div class="flex items-start justify-between gap-3">
							<input
								type="checkbox"
								checked={selectedTestSlugs.has(req.test_slug)}
								onchange={() => toggleTestSelection(req.test_slug)}
								disabled={selectedTestSlugs.size >= 4 && !selectedTestSlugs.has(req.test_slug)}
								aria-label={t('language_test.select_test_for_compare', 'Select {name} for comparison').replace('{name}', req.test_name)}
								class="mt-1 accent-[var(--color-primary)]"
							/>
							<div class="min-w-0 flex-1">
								<button
									type="button"
									onclick={() => selectTest(req.test_slug)}
									class="text-sm font-semibold text-[var(--color-text)] hover:text-[var(--color-primary)] transition-colors text-start"
								>
									{req.test_name}
								</button>
								<div class="flex items-center gap-2 flex-wrap mt-1">
									<span class="text-xs px-2 py-0.5 bg-[var(--color-surface-subtle)] text-[var(--color-primary)] rounded-full font-semibold">
										{t('language_test.min_score', 'Min score:')} {req.minimum_score}
									</span>
									{#if req.visa_category}
										<span class="text-xs text-[var(--color-muted)]">{req.visa_category}</span>
									{/if}
								</div>
							</div>
							<button
								type="button"
								onclick={() => selectTest(req.test_slug)}
								class="text-[var(--color-muted)] hover:text-[var(--color-text)] flex-shrink-0"
								aria-label={t('language_test.view_test', 'View test details')}
							>
								<ChevronRight class="rtl:-scale-x-100" size={20} />
							</button>
						</div>

						{#if req.score_breakdown}
							<dl class="mt-3 space-y-1">
								{#each breakdownEntries(req.score_breakdown) as [key, val] (key)}
									<div class="flex items-start gap-2 text-sm">
										<dt class="font-medium text-[var(--color-text)] flex-shrink-0">{key}:</dt>
										<dd class="text-[var(--color-text-subtle)]">{breakdownValue(val)}</dd>
									</div>
								{/each}
							</dl>
						{/if}

						{#if req.notes}
							<p class="mt-2 text-sm text-[var(--color-text-subtle)]">{req.notes}</p>
						{/if}

						{#if req.source_url}
							<a
								href={req.source_url}
								target="_blank"
								rel="noopener noreferrer"
								class="mt-2 inline-flex items-center gap-1 text-xs text-[var(--color-primary)] hover:underline"
							>
								<ExternalLink class="rtl:-scale-x-100" size={12} />
								{t('language_test.source', 'Source')}
							</a>
						{/if}
					</div>
				{/each}
			</div>
		{/if}

		<!-- All tests quick-access -->
		{#if tests.length > 0 && !selectedDestId}
			<div class="border-t border-[var(--color-border)] px-4 py-4">
				<div class="mb-3 flex items-center justify-between flex-wrap gap-3">
					<h3 class="text-sm font-semibold text-[var(--color-muted)] uppercase tracking-wide">
						{t('language_test.all_tests', 'All language tests')}
					</h3>
					<form
						method="POST"
						use:enhance={() => {
							savingJCE = true;
							return async ({ update }) => {
								savingJCE = false;
								await update();
							};
						}}
						class="contents"
					>
						{#each Array.from(selectedTestSlugs) as slug (slug)}
							<input type="hidden" name="test_slug" value={slug} />
						{/each}
						<button
							type="submit"
							disabled={!canCompare}
							onclick={(e) => {
								if (!authed) {
									e.preventDefault();
									showRegisterPrompt = true;
								}
							}}
							title={authed
								? t('language_test.compare_in_jce_hint', 'Compare in JCE')
								: t('language_test.sign_in_to_compare', 'Sign in to compare')}
							class="inline-flex items-center gap-2 px-4 py-2 bg-[var(--color-primary)] text-[var(--color-primary-contrast)] rounded-[var(--radius)] text-sm font-medium hover:bg-[var(--color-primary-hover)] transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
						>
							{#if savingJCE}
								<Loader2 size={16} class="animate-spin" />
							{:else}
								<GitCompare size={16} />
							{/if}
							{t('language_test.compare_in_jce', 'Compare in JCE')}
						</button>
					</form>
				</div>

				{#if form?.error}
					<div class="mb-4 rounded-[var(--radius)] border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300">
						{form.message || t('language_test.jce_error', 'Could not create comparison')}
					</div>
				{/if}

				<div class="flex flex-wrap gap-2">
					{#each tests as test (test.slug)}
						<div class="inline-flex items-center">
							<label
								class="inline-flex items-center gap-2 px-3 py-1.5 text-sm font-medium text-[var(--color-text)] bg-[var(--color-surface)] border border-[var(--color-border)] rounded-s-[var(--radius)] border-e-0 hover:border-[var(--color-primary)] hover:text-[var(--color-primary)] transition-colors cursor-pointer {selectedTestSlugs.has(test.slug) ? 'border-[var(--color-primary)] ring-1 ring-[var(--color-primary)]' : ''}"
							>
								<input
									type="checkbox"
									checked={selectedTestSlugs.has(test.slug)}
									onchange={() => toggleTestSelection(test.slug)}
									disabled={selectedTestSlugs.size >= 4 && !selectedTestSlugs.has(test.slug)}
									class="accent-[var(--color-primary)]"
								/>
								{test.name}
							</label>
							<button
								type="button"
								onclick={() => selectTest(test.slug)}
								class="px-2 py-1.5 text-[var(--color-muted)] bg-[var(--color-surface)] border border-[var(--color-border)] rounded-e-[var(--radius)] hover:text-[var(--color-text)] transition-colors"
								aria-label={t('language_test.view_test', 'View test details')}
							>
								<ChevronRight class="rtl:-scale-x-100" size={16} />
							</button>
						</div>
					{/each}
				</div>
			</div>
		{/if}
	{/if}

	{#if showRegisterPrompt}
		<div class="p-4">
			<div class="mb-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-xs text-[var(--color-muted)]">
				{t('language_test.sign_in_to_save', 'Create a free account to share your score story or save comparisons.')}
				<a href="/login" class="ms-1 text-[var(--color-primary)] hover:underline"
					>{t('common.sign_in', 'Sign in')}</a
				>.
			</div>
		</div>
	{/if}
</div>

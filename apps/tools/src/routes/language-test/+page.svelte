<script lang="ts">
	import { untrack } from 'svelte';
	import { replaceState } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { buildSeoMeta } from '$lib/seo';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import LoadingSkeleton from '$lib/components/ui/LoadingSkeleton.svelte';
	import RegistrationPrompt from '$lib/components/ui/RegistrationPrompt.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import DestinationSelector from '$lib/components/tools/DestinationSelector.svelte';
	import type {
		LanguageTest,
		LanguageTestRequirementWithAi,
		LanguageTestResource,
		LanguageTestScoreStory,
		GeoSpaceOption
	} from '$lib/types';

	/**
	 * Language Test Hub tool — §99, ADR-088.
	 * Two modes:
	 * 1. Requirements mode (default): Destination selector → optional visa
	 *    category → list of requirements. Each requirement links to test detail.
	 * 2. Test detail mode (when a test is selected): test info, resources list,
	 *    score stories, and a story submission form for authed users.
	 */
	let { data } = $props();

	let destinations = $state<GeoSpaceOption[]>(untrack(() => data.destinations));
	let tests = $state<LanguageTest[]>(untrack(() => data.tests));
	let requirements = $state<LanguageTestRequirementWithAi[]>(untrack(() => data.requirements));
	let user = $state(untrack(() => data.user));

	let selectedDestId = $state<string>(untrack(() => data.initialDestId ?? ''));
	let visaCategory = $state<string>('');
	let loadingRequirements = $state(false);

	// Test detail mode
	let selectedTestSlug = $state<string | null>(null);
	let testDetail = $state<LanguageTest | null>(null);
	let testResources = $state<LanguageTestResource[]>([]);
	let testStories = $state<LanguageTestScoreStory[]>([]);
	let loadingDetail = $state(false);

	// Story submission form
	let showRegisterPrompt = $state(false);
	let storyFinalScore = $state<string>('');
	let storyStartingLevel = $state<string>('');
	let storyPrepWeeks = $state<string>('');
	let storyResourcesUsed = $state<string>('');
	let storyAdvice = $state<string>('');
	let submittingStory = $state(false);
	let storyError = $state<string | null>(null);

	const seo = $derived(
		buildSeoMeta({
			title: t('language_test.seo_title', 'Language Test Hub — JapaConnect Tools'),
			description: t(
				'language_test.seo_description',
				'Find required language tests for your visa, minimum scores, prep resources, and community score stories.'
			),
			path: '/language-test',
			origin: page.url.origin
		})
	);

	async function loadRequirements() {
		if (!selectedDestId) {
			requirements = [];
			return;
		}
		loadingRequirements = true;
		const params = new URLSearchParams();
		params.set('dest', selectedDestId);
		if (visaCategory) params.set('visa', visaCategory);
		const res = await api.get<LanguageTestRequirementWithAi[]>(
			`/language-tests/requirements?${params.toString()}`
		);
		loadingRequirements = false;
		if (!res.error) requirements = res.data ?? [];
		else requirements = [];
	}

	function onDestChange(id: string) {
		selectedTestSlug = null;
		loadRequirements();
		const url = new URL(page.url);
		if (id) url.searchParams.set('dest', id);
		else url.searchParams.delete('dest');
		replaceState(url, {});
	}

	function onVisaInput() {
		loadRequirements();
	}

	/** Enter test detail mode — fetch test info, resources, and stories. */
	async function selectTest(slug: string) {
		selectedTestSlug = slug;
		loadingDetail = true;
		testDetail = null;
		testResources = [];
		testStories = [];

		// Fetch test detail (nested data object: { test, resources_summary, stories_summary })
		const detailRes = await api.get<{
			test: LanguageTest;
			resources_summary: LanguageTestResource[];
			stories_summary: LanguageTestScoreStory[];
		}>(`/language-tests/${slug}`);
		if (!detailRes.error && detailRes.data) {
			testDetail = detailRes.data.test;
			testResources = detailRes.data.resources_summary ?? [];
			testStories = detailRes.data.stories_summary ?? [];
		}

		// Fetch full resources list
		const resRes = await api.get<LanguageTestResource[]>(
			`/language-tests/${slug}/resources`
		);
		if (!resRes.error && resRes.data) {
			testResources = resRes.data;
		}

		// Fetch stories (optionally filtered by destination)
		const storyParams = new URLSearchParams();
		if (selectedDestId) storyParams.set('dest', selectedDestId);
		const storyRes = await api.get<LanguageTestScoreStory[]>(
			`/language-tests/${slug}/stories?${storyParams.toString()}`
		);
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
	}

	function openStoryForm() {
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		storyFinalScore = '';
		storyStartingLevel = '';
		storyPrepWeeks = '';
		storyResourcesUsed = '';
		storyAdvice = '';
		storyError = null;
	}

	async function submitStory() {
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		if (!selectedTestSlug) return;
		submittingStory = true;
		storyError = null;
		const body: Record<string, unknown> = {
			final_score: storyFinalScore
		};
		if (storyStartingLevel) body.starting_level = storyStartingLevel;
		if (selectedDestId) body.destination_space_id = selectedDestId;
		if (storyPrepWeeks) body.prep_duration_weeks = parseInt(storyPrepWeeks, 10);
		if (storyResourcesUsed) {
			body.resources_used = storyResourcesUsed
				.split(',')
				.map((s) => s.trim())
				.filter(Boolean);
		}
		if (storyAdvice) body.advice = storyAdvice;
		const res = await api.post<LanguageTestScoreStory>(
			`/language-tests/${selectedTestSlug}/stories`,
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
		storyPrepWeeks = '';
		storyResourcesUsed = '';
		storyAdvice = '';
	}

	/** Get score_breakdown entries from a JSON object field. */
	function breakdownEntries(
		val: Record<string, unknown> | null | undefined
	): [string, unknown][] {
		if (!val || typeof val !== 'object') return [];
		return Object.entries(val);
	}

	/** Format an ISO date string as a readable date. */
	function formatDate(iso?: string | null): string {
		if (!iso) return '';
		try {
			return new Date(iso).toLocaleDateString(undefined, {
				year: 'numeric',
				month: 'short',
				day: 'numeric'
			});
		} catch {
			return iso;
		}
	}
</script>

<svelte:head>
	<title>{seo.title}</title>
	<meta name="description" content={seo.description} />
	<link rel="canonical" href={seo.canonical} />
	<meta property="og:title" content={seo.title} />
	<meta property="og:description" content={seo.description} />
	<meta property="og:url" content={seo.canonical} />
	<meta property="og:type" content="website" />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={seo.title} />
	<meta name="twitter:description" content={seo.description} />
</svelte:head>

<div class="mx-auto max-w-4xl px-4 py-8">
	{#if !data.enabled}
		<ToolDisabled tool={data.tool} />
	{:else}
	<div class="mb-8">
		<h1 class="text-2xl sm:text-3xl font-bold text-text">
			{t('language_test.title', 'Language Test Hub')}
		</h1>
		<p class="mt-2 text-text-subtle">
			{t(
				'language_test.subtitle',
				'Find required language tests for your visa, minimum scores, prep resources, and community score stories.'
			)}
		</p>
	</div>

	{#if destinations.length === 0}
		<EmptyState
			icon="languages"
			title={t('language_test.no_destinations', 'No destinations available')}
			description={t(
				'language_test.no_destinations_desc',
				"Language test data hasn't been configured yet. Please check back later."
			)}
		/>
	{:else if selectedTestSlug}
		<!-- ===== Test detail mode ===== -->
		<div class="space-y-6">
			<button
				onclick={backToRequirements}
				class="inline-flex items-center gap-1.5 text-sm text-muted hover:text-text transition-colors"
			>
				<Icon name="arrow-left" size={16} />
				{t('language_test.back_to_requirements', 'Back to requirements')}
			</button>

			{#if loadingDetail}
				<div class="space-y-3">
					<LoadingSkeleton height="60px" />
					<LoadingSkeleton height="120px" />
					<LoadingSkeleton height="120px" />
				</div>
			{:else if testDetail}
				<!-- Test info -->
				<div class="p-4 bg-surface border border-border rounded-lg">
					<div class="flex items-center gap-2 flex-wrap">
						<h2 class="text-xl font-semibold text-text">{testDetail.name}</h2>
						{#if testDetail.validity_months}
							<span class="text-xs px-2 py-0.5 bg-[var(--tool-accent)]/10 text-[var(--tool-accent)] rounded-full font-medium">
								{t('language_test.valid_for', 'Valid for')} {testDetail.validity_months} {t('language_test.months', 'months')}
							</span>
						{/if}
					</div>
					{#if testDetail.official_url}
						<a
							href={testDetail.official_url}
							target="_blank"
							rel="noopener noreferrer"
							class="mt-2 inline-flex items-center gap-1.5 text-sm text-primary hover:underline"
						>
							<Icon name="external-link" size={14} />
							{t('language_test.official_website', 'Official website')}
						</a>
					{/if}
				</div>

				<!-- Resources -->
				<div>
					<h3 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
						{t('language_test.resources', 'Prep resources')}
					</h3>
					{#if testResources.length === 0}
						<EmptyState
							icon="book-open"
							title={t('language_test.no_resources', 'No resources')}
							description={t(
								'language_test.no_resources_desc',
								'No prep resources have been added for this test yet.'
							)}
						/>
					{:else}
						<div class="space-y-2">
							{#each testResources as resource (resource.id)}
								<div class="p-3 bg-surface border border-border rounded-lg flex items-center justify-between gap-3">
									<div class="min-w-0">
										<a
											href={resource.resource_url}
											target="_blank"
											rel="noopener noreferrer"
											class="text-sm font-medium text-text hover:text-primary transition-colors"
										>
											{resource.title}
										</a>
										<div class="flex items-center gap-2 text-xs text-muted mt-0.5">
											{#if resource.resource_type}
												<span>{resource.resource_type}</span>
											{/if}
											{#if resource.community_rating}
												<span class="flex items-center gap-0.5">
													<Icon name="star" size={12} class="text-yellow-500" />
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
										class="text-muted hover:text-text flex-shrink-0"
										aria-label={t('language_test.open_resource', 'Open resource')}
									>
										<Icon name="external-link" size={16} />
									</a>
								</div>
							{/each}
						</div>
					{/if}
				</div>

				<!-- Score stories -->
				<div>
					<div class="flex items-center justify-between mb-3">
						<h3 class="text-sm font-semibold text-muted uppercase tracking-wide">
							{t('language_test.score_stories', 'Community score stories')}
						</h3>
						<button
							onclick={openStoryForm}
							class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium text-white bg-[var(--tool-accent)] rounded-lg hover:bg-[var(--tool-accent-hover)] transition-colors"
						>
							<Icon name="plus" size={16} />
							{t('language_test.share_story', 'Share your story')}
						</button>
					</div>

					<!-- Story submission form -->
					{#if user && !showRegisterPrompt}
						<div class="mb-4 p-4 bg-surface-subtle border border-border rounded-lg space-y-3">
							{#if storyError}
								<p class="text-sm text-red-500">{storyError}</p>
							{/if}
							<div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
								<div>
									<label for="story-score" class="block text-xs font-medium text-muted mb-1">
										{t('language_test.final_score', 'Final score')} *
									</label>
									<input
										id="story-score"
										type="text"
										bind:value={storyFinalScore}
										placeholder="e.g. 7.5"
										class="w-full px-3 py-2 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
									/>
								</div>
								<div>
									<label for="story-level" class="block text-xs font-medium text-muted mb-1">
										{t('language_test.starting_level', 'Starting level')}
									</label>
									<input
										id="story-level"
										type="text"
										bind:value={storyStartingLevel}
										placeholder="e.g. B1"
										class="w-full px-3 py-2 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
									/>
								</div>
								<div>
									<label for="story-weeks" class="block text-xs font-medium text-muted mb-1">
										{t('language_test.prep_weeks', 'Prep duration (weeks)')}
									</label>
									<input
										id="story-weeks"
										type="number"
										bind:value={storyPrepWeeks}
										min="0"
										class="w-full px-3 py-2 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
									/>
								</div>
								<div>
									<label for="story-resources" class="block text-xs font-medium text-muted mb-1">
										{t('language_test.resources_used', 'Resources used (comma-separated)')}
									</label>
									<input
										id="story-resources"
										type="text"
										bind:value={storyResourcesUsed}
										placeholder="e.g. IELTS Liz, YouTube, Cambridge books"
										class="w-full px-3 py-2 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
									/>
								</div>
							</div>
							<div>
								<label for="story-advice" class="block text-xs font-medium text-muted mb-1">
									{t('language_test.advice', 'Advice')}
								</label>
								<textarea
									id="story-advice"
									bind:value={storyAdvice}
									rows="3"
									placeholder={t('language_test.advice_placeholder', 'Share your prep advice for others...')}
									class="w-full px-3 py-2 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
								></textarea>
							</div>
							<button
								onclick={submitStory}
								disabled={submittingStory || !storyFinalScore}
								class="px-4 py-2 text-sm font-medium text-white bg-[var(--tool-accent)] rounded-lg hover:bg-[var(--tool-accent-hover)] transition-colors disabled:opacity-50"
							>
								{submittingStory
									? t('common.submitting', 'Submitting...')
									: t('common.submit', 'Submit')}
							</button>
						</div>
					{/if}

					{#if testStories.length === 0}
						<EmptyState
							icon="message-square"
							title={t('language_test.no_stories', 'No score stories')}
							description={t(
								'language_test.no_stories_desc',
								'No community score stories have been shared for this test yet.'
							)}
						/>
					{:else}
						<div class="space-y-2">
							{#each testStories as story (story.id)}
								<div class="p-4 bg-surface border border-border rounded-lg">
									<div class="flex items-center gap-2 flex-wrap mb-2">
										{#if story.author_name}
											<span class="text-sm font-medium text-text">{story.author_name}</span>
										{/if}
										<span class="text-xs px-2 py-0.5 bg-[var(--tool-accent)]/10 text-[var(--tool-accent)] rounded-full font-semibold">
											{t('language_test.score', 'Score:')} {story.final_score}
										</span>
										{#if story.starting_level}
											<span class="text-xs text-muted">
												{t('language_test.from', 'from')} {story.starting_level}
											</span>
										{/if}
									</div>
									<div class="flex items-center gap-3 text-xs text-muted mb-2">
										{#if story.prep_duration_weeks}
											<span class="flex items-center gap-1">
												<Icon name="clock" size={12} />
												{story.prep_duration_weeks} {t('language_test.weeks', 'weeks')}
											</span>
										{/if}
										{#if story.destination_space_name}
											<span class="flex items-center gap-1">
												<Icon name="map-pin" size={12} />
												{story.destination_space_name}
											</span>
										{/if}
										<span>{formatDate(story.created_at)}</span>
									</div>
									{#if story.resources_used && story.resources_used.length > 0}
										<div class="flex items-center gap-1.5 flex-wrap mb-2">
											<span class="text-xs text-muted">{t('language_test.resources', 'Resources:')}</span>
											{#each story.resources_used as res (res)}
												<span class="text-xs px-1.5 py-0.5 bg-surface-subtle rounded">{res}</span>
											{/each}
										</div>
									{/if}
									{#if story.advice}
										<p class="text-sm text-text-subtle">{story.advice}</p>
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
		<div class="mb-4">
			<DestinationSelector
				{destinations}
				bind:selectedId={selectedDestId}
				onchange={onDestChange}
			/>
		</div>

		{#if selectedDestId}
			<!-- Optional visa category filter -->
			<div class="mb-6 flex items-center gap-3">
				<label for="visa-cat" class="text-sm font-medium text-text whitespace-nowrap">
					{t('language_test.visa_category', 'Visa category:')}
				</label>
				<input
					id="visa-cat"
					type="text"
					bind:value={visaCategory}
					oninput={onVisaInput}
					placeholder={t('language_test.visa_placeholder', 'e.g. skilled worker (optional)')}
					class="flex-1 px-4 py-2 bg-surface border border-border rounded-lg text-text focus:border-primary focus:outline-none"
				/>
			</div>
		{/if}

		{#if loadingRequirements}
			<div class="space-y-3">
				<LoadingSkeleton height="80px" />
				<LoadingSkeleton height="80px" />
				<LoadingSkeleton height="80px" />
			</div>
		{:else if !selectedDestId}
			<EmptyState
				icon="map-pin"
				title={t('language_test.select_destination', 'Select a destination')}
				description={t(
					'language_test.select_destination_desc',
					'Choose a country to see language test requirements for that destination.'
				)}
			/>
		{:else if requirements.length === 0}
			<EmptyState
				icon="languages"
				title={t('language_test.no_requirements', 'No requirements')}
				description={t(
					'language_test.no_requirements_desc',
					'No language test requirements have been published for this destination yet.'
				)}
			/>
		{:else}
			<div class="space-y-3">
				{#each requirements as req (req.id)}
					<div class="p-4 bg-surface border border-border rounded-lg">
						<div class="flex items-center justify-between gap-3">
							<div class="min-w-0">
								<button
									onclick={() => selectTest(req.test_slug)}
									class="text-sm font-semibold text-text hover:text-primary transition-colors text-left"
								>
									{req.test_name}
								</button>
								<div class="flex items-center gap-2 flex-wrap mt-1">
									<span class="text-xs px-2 py-0.5 bg-[var(--tool-accent)]/10 text-[var(--tool-accent)] rounded-full font-semibold">
										{t('language_test.min_score', 'Min score:')} {req.minimum_score}
									</span>
									{#if req.visa_category}
										<span class="text-xs text-muted">{req.visa_category}</span>
									{/if}
								</div>
							</div>
							<button
								onclick={() => selectTest(req.test_slug)}
								class="text-muted hover:text-text flex-shrink-0"
								aria-label={t('language_test.view_test', 'View test details')}
							>
								<Icon name="chevron-right" size={20} />
							</button>
						</div>

						{#if req.score_breakdown}
							<dl class="mt-3 space-y-1">
								{#each breakdownEntries(req.score_breakdown) as [key, val] (key)}
									<div class="flex items-start gap-2 text-sm">
										<dt class="font-medium text-text flex-shrink-0">{key}:</dt>
										<dd class="text-text-subtle">{String(val)}</dd>
									</div>
								{/each}
							</dl>
						{/if}

						{#if req.notes}
							<p class="mt-2 text-sm text-text-subtle">{req.notes}</p>
						{/if}

						{#if req.source_url}
							<a
								href={req.source_url}
								target="_blank"
								rel="noopener noreferrer"
								class="mt-2 inline-flex items-center gap-1 text-xs text-primary hover:underline"
							>
								<Icon name="external-link" size={12} />
								{t('language_test.source', 'Source')}
							</a>
						{/if}
					</div>
				{/each}
			</div>
		{/if}

		<!-- All tests quick-access -->
		{#if tests.length > 0 && !selectedDestId}
			<div class="mt-8">
				<h3 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
					{t('language_test.all_tests', 'All language tests')}
				</h3>
				<div class="flex flex-wrap gap-2">
					{#each tests as test (test.slug)}
						<button
							onclick={() => selectTest(test.slug)}
							class="px-3 py-1.5 text-sm font-medium text-text bg-surface border border-border rounded-lg hover:border-primary hover:text-primary transition-colors"
						>
							{test.name}
						</button>
					{/each}
				</div>
			</div>
		{/if}
	{/if}

	{#if showRegisterPrompt}
		<RegistrationPrompt
			open={showRegisterPrompt}
			onClose={() => (showRegisterPrompt = false)}
			icon="message-square-plus"
			title={t('register.share_story', 'Share your score story')}
			description={t(
				'register.share_story_desc',
				'Create a free account to share your language test experience and help others prepare.'
			)}
		/>
	{/if}
	{/if}
</div>

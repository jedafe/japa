<script lang="ts">
	import { untrack } from 'svelte';
	import { replaceState } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { buildSeoMeta, mainAppUrl } from '$lib/seo';
	import SeoHead from '$lib/components/seo/SeoHead.svelte';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import LoadingSkeleton from '$lib/components/ui/LoadingSkeleton.svelte';
	import RegistrationPrompt from '$lib/components/ui/RegistrationPrompt.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import type { Scholarship, SavedScholarship, ScholarshipStatus, GeoSpaceOption, CatalogOption } from '$lib/types';
	import { t, siteName, i18n } from '$lib/i18n';

	/**
	 * Scholarship Finder tool — §73, ADR-073.
	 * Filter panel → results list → save-to-tracker with application status.
	 * Anonymous users can browse; saving requires auth.
	 */
	let { data } = $props();

	let scholarships = $state<Scholarship[]>(untrack(() => data.scholarships));
	let saved = $state<SavedScholarship[]>(untrack(() => data.saved));
	let user = $state(untrack(() => data.user));
	let countries = $state<GeoSpaceOption[]>(untrack(() => data.countries));
	let statusOptions = $state<CatalogOption[]>(untrack(() => data.statusOptions));
	let renewableOptions = $state<CatalogOption[]>(untrack(() => data.renewableOptions));

	// Filter state — initialized from URL so pagination and shareable links work.
	const initialParams = page.url.searchParams;
	let filterCountry = $state(initialParams.get('country') ?? '');
	let filterLevel = $state(initialParams.get('level') ?? '');
	let filterMinAmount = $state(initialParams.get('min_amount') ?? '');
	let filterMaxAmount = $state(initialParams.get('max_amount') ?? '');
	let filterRenewable = $state(initialParams.get('renewable') ?? '');
	let filterDeadlineFrom = $state(initialParams.get('deadline_from') ?? '');
	let filterDeadlineTo = $state(initialParams.get('deadline_to') ?? '');
	let filterQuery = $state(initialParams.get('q') ?? '');
	let filterSort = $state(initialParams.get('sort') ?? '');
	let filterIncludeExpired = $state(initialParams.get('include_expired') === 'true');
	let loading = $state(false);
	let showRegisterPrompt = $state(false);

	// Pagination state from SSR metadata.
	let currentPage = $state(untrack(() => data.currentPage ?? 1));
	let pageSize = $state(untrack(() => data.pageSize ?? 20));
	let total = $state(untrack(() => data.total ?? 0));
	const totalPages = $derived(Math.max(1, Math.ceil(total / (pageSize || 1))));
	const showingStart = $derived((currentPage - 1) * pageSize + 1);
	const showingEnd = $derived(Math.min(currentPage * pageSize, total));

	const seo = $derived(
		buildSeoMeta({
			title: t('scholarships.seo_title', 'Scholarship Finder — {site} Tools'),
			description: t(
				'scholarships.seo_description',
				'Filter scholarships by country, level of study, award amount, and deadline. Save scholarships to your tracker and get deadline reminders.'
			),
			path: '/scholarships',
			origin: page.url.origin,
			ogImage: `${page.url.origin}/og/og-scholarships.png`,
			jsonLd: {
				'@context': 'https://schema.org',
				'@type': 'EducationalOccupationalProgram',
				name: `${siteName()} Scholarship Finder`,
				description: 'Find scholarships for studying abroad.'
			}
		})
	);

	let amountError = $state('');

	async function applyFilters(resetPage = true) {
		amountError = '';
		if (filterMinAmount !== '' && Number.isNaN(Number(filterMinAmount))) {
			amountError = t('scholarships.invalid_amount', 'Min and max amounts must be numbers.');
			return;
		}
		if (filterMaxAmount !== '' && Number.isNaN(Number(filterMaxAmount))) {
			amountError = t('scholarships.invalid_amount', 'Min and max amounts must be numbers.');
			return;
		}
		if (filterMinAmount !== '' && filterMaxAmount !== '' && Number(filterMinAmount) > Number(filterMaxAmount)) {
			amountError = t('scholarships.invalid_amount_range', 'Min amount cannot exceed max amount.');
			return;
		}
		loading = true;
		if (resetPage) currentPage = 1;
		const params = new URLSearchParams();
		if (filterCountry) params.set('country', filterCountry);
		if (filterLevel) params.set('level', filterLevel);
		if (filterMinAmount) params.set('min_amount', filterMinAmount);
		if (filterMaxAmount) params.set('max_amount', filterMaxAmount);
		if (filterRenewable) params.set('renewable', filterRenewable);
		if (filterDeadlineFrom) params.set('deadline_from', filterDeadlineFrom);
		if (filterDeadlineTo) params.set('deadline_to', filterDeadlineTo);
		if (filterQuery) params.set('q', filterQuery);
		if (filterSort) params.set('sort', filterSort);
		if (filterIncludeExpired) params.set('include_expired', 'true');
		params.set('page', String(currentPage));
		params.set('page_size', String(pageSize));
		const qs = params.toString();
		const res = await api.getList<Scholarship>(`/scholarships${qs ? `?${qs}` : ''}`);
		loading = false;
		if (res.error) return;
		scholarships = res.data ?? [];
		total = res.meta?.total ?? res.data?.length ?? 0;
		// Update URL for shareability (SvelteKit replaceState, not window.history)
		const url = new URL(page.url);
		url.search = qs;
		replaceState(url, {});
	}

	async function changePage(newPage: number) {
		if (newPage < 1 || newPage > totalPages) return;
		currentPage = newPage;
		await applyFilters(false);
	}

	function isSaved(guideId: string): SavedScholarship | undefined {
		return saved.find((s) => s.scholarship.guide_id === guideId);
	}

	async function toggleSave(scholarship: Scholarship) {
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		const existing = isSaved(scholarship.guide_id);
		if (existing) {
			const res = await api.delete(`/scholarships/${scholarship.guide_id}/save`);
			if (!res.error) {
				saved = saved.filter((s) => s.scholarship.guide_id !== scholarship.guide_id);
			}
		} else {
			const res = await api.post<{ id: string }>(`/scholarships/${scholarship.guide_id}/save`);
			if (!res.error && res.data) {
				saved = [
					...saved,
					{
						id: res.data.id,
						scholarship,
						status: 'saved' as ScholarshipStatus,
						notes: null,
						remind_before_days: data.defaultReminderDays,
						last_reminded_at: null,
						created_at: new Date().toISOString(),
						updated_at: new Date().toISOString()
					}
				];
			}
		}
	}

	async function updateStatus(guideId: string, status: ScholarshipStatus) {
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		const res = await api.patch(`/scholarships/${guideId}/save`, { status });
		if (!res.error) {
			saved = saved.map((s) =>
				s.scholarship.guide_id === guideId ? { ...s, status } : s
			);
		}
	}

	function formatAmount(amount: number | null | undefined): string {
		if (amount === null || amount === undefined) return '—';
		return new Intl.NumberFormat('en-US', {
			style: 'currency',
			currency: 'USD',
			maximumFractionDigits: 0
		}).format(amount);
	}

	function formatDate(date: string | null | undefined): string {
		if (!date) return '—';
		return new Date(date).toLocaleDateString(i18n.locale, {
			year: 'numeric',
			month: 'short',
			day: 'numeric'
		});
	}

	function daysUntilDeadline(deadline: string | null | undefined): number | null {
		if (!deadline) return null;
		const diff = new Date(deadline).getTime() - Date.now();
		return Math.ceil(diff / (1000 * 60 * 60 * 24));
	}

	// Scholarship application status options are loaded from
	// GET /scholarships/statuses (Rule 16 — no hardcoded option lists).
	// Labels are translated client-side using value-based i18n keys.

	// Level-of-study filter options come from GET /scholarships/levels (the
	// canonical distinct set across all published scholarships — Rule 16).
	// Falls back to deriving from the loaded page when the endpoint is empty.
	const levelOptions = $derived.by(() => {
		const apiLevels = (data.levelOptions ?? []).map((o) => o.value);
		if (apiLevels.length > 0) return apiLevels;
		const levels = new Set<string>();
		for (const s of scholarships) {
			if (s.level_of_study) levels.add(s.level_of_study);
		}
		return Array.from(levels).sort();
	});

	function fetchLevels(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		const filtered = query === ''
			? levelOptions
			: levelOptions.filter((l) => l.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((l) => ({ id: l, label: l })));
	}

	function fetchLevelsInitial(): Promise<AutocompleteItem[]> {
		return fetchLevels('');
	}

	function fetchCountries(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		const filtered =
			query === ''
				? countries
				: countries.filter((c) => c.name.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((c) => ({ id: c.name, label: c.name })));
	}

	function fetchCountriesInitial(): Promise<AutocompleteItem[]> {
		return fetchCountries('');
	}
</script>

<SeoHead seo={seo} ogType="website" twitterCard="summary" />

<div class="mx-auto max-w-5xl px-4 py-8">
	{#if !data.enabled}
		<ToolDisabled tool={data.tool} />
	{:else}
	<div class="mb-8 flex items-start justify-between gap-4">
		<div>
			<h1 class="text-2xl sm:text-3xl font-bold text-text">{t('scholarships.title', 'Scholarship Finder')}</h1>
			<p class="mt-2 text-text-subtle">
				{t('scholarships.subtitle', 'Find funding for your studies abroad. Filter, save, and track your applications.')}
			</p>
		</div>
		{#if user}
			<a
				href="/scholarships/saved"
				class="inline-flex items-center gap-1.5 px-3 py-2 text-sm font-medium text-text border border-border rounded-lg hover:border-[var(--tool-accent)] hover:text-[var(--tool-accent)] transition-colors flex-shrink-0"
			>
				<Icon name="bookmark" size={16} />
				<span class="hidden sm:inline">{t('scholarships.my_saved', 'My saved')}</span>
			</a>
		{/if}
	</div>

	<div class="grid grid-cols-1 lg:grid-cols-[280px_1fr] gap-6">
		<!-- Filter panel -->
		<aside class="space-y-4">
			<div class="p-4 bg-surface border border-border rounded-lg space-y-4">
				<h2 class="text-sm font-semibold text-text">{t('scholarships.filters', 'Filters')}</h2>

				<div>
					<label class="block text-xs font-medium text-muted mb-1" for="scholarship-search">{t('scholarships.search', 'Search')}</label>
					<input
						id="scholarship-search"
						type="search"
						bind:value={filterQuery}
						onkeydown={(e) => e.key === 'Enter' && applyFilters()}
						placeholder={t('scholarships.search_placeholder', 'Name, institution...')}
						class="w-full px-3 py-2 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
					/>
				</div>

				<div>
					<label class="block text-xs font-medium text-muted mb-1" for="scholarship-country">{t('scholarships.country', 'Country')}</label>
					<Autocomplete
						id="scholarship-country"
						placeholder={t('scholarships.country_placeholder', 'e.g. United Kingdom')}
						emptyMessage={t('geo.no_destinations', 'No countries found')}
						fetcher={fetchCountries}
						initialFetcher={fetchCountriesInitial}
						selectedId={filterCountry || null}
						selectedLabel={filterCountry || null}
						onSelect={(item) => (filterCountry = item.id)}
						onClear={() => (filterCountry = '')}
					/>
				</div>

				<div>
					<label class="block text-xs font-medium text-muted mb-1" for="scholarship-level">{t('scholarships.level', 'Level of study')}</label>
					<Autocomplete
						id="scholarship-level"
						placeholder={t('scholarships.level_any', 'Any level')}
						emptyMessage={t('scholarships.no_levels', 'No levels found')}
						fetcher={fetchLevels}
						initialFetcher={fetchLevelsInitial}
						selectedId={filterLevel || null}
						selectedLabel={filterLevel || null}
						onSelect={(item) => (filterLevel = item.id)}
						onClear={() => (filterLevel = '')}
					/>
				</div>

				<div class="grid grid-cols-2 gap-2">
					<div>
						<label class="block text-xs font-medium text-muted mb-1" for="scholarship-min-amount">{t('scholarships.min_amount', 'Min $')}</label>
						<input
							id="scholarship-min-amount"
							type="number"
							bind:value={filterMinAmount}
							placeholder="0"
							class="w-full px-3 py-2 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
						/>
					</div>
					<div>
						<label class="block text-xs font-medium text-muted mb-1" for="scholarship-max-amount">{t('scholarships.max_amount', 'Max $')}</label>
						<input
							id="scholarship-max-amount"
							type="number"
							bind:value={filterMaxAmount}
							placeholder="any"
							class="w-full px-3 py-2 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
						/>
					</div>
				</div>

				<div>
					<label class="block text-xs font-medium text-muted mb-1" for="scholarship-renewable">{t('scholarships.renewable', 'Renewable')}</label>
					<AutocompleteSelect
						id="scholarship-renewable"
						bind:value={filterRenewable}
						options={renewableOptions.map((opt) => ({
							value: opt.value,
							label:
								opt.value === 'true'
									? t('scholarships.renewable_yes', opt.label)
									: opt.value === 'false'
										? t('scholarships.renewable_no', opt.label)
										: t('scholarships.renewable_any', opt.label)
						}))}
						class="w-full"
					/>
				</div>

				<div class="grid grid-cols-2 gap-2">
					<div>
						<label class="block text-xs font-medium text-muted mb-1" for="scholarship-deadline-from">{t('scholarships.deadline_from', 'Deadline from')}</label>
						<input
							id="scholarship-deadline-from"
							type="date"
							bind:value={filterDeadlineFrom}
							class="w-full px-3 py-2 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
						/>
					</div>
					<div>
						<label class="block text-xs font-medium text-muted mb-1" for="scholarship-deadline-to">{t('scholarships.deadline_to', 'Deadline to')}</label>
						<input
							id="scholarship-deadline-to"
							type="date"
							bind:value={filterDeadlineTo}
							class="w-full px-3 py-2 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
						/>
					</div>
				</div>

				<div>
					<label class="block text-xs font-medium text-muted mb-1" for="scholarship-sort">{t('scholarships.sort', 'Sort')}</label>
					<AutocompleteSelect
						id="scholarship-sort"
						bind:value={filterSort}
						options={[
							{ value: '', label: t('scholarships.sort_default', 'Default') },
							{ value: 'deadline_asc', label: t('scholarships.sort_deadline', 'Deadline (soonest)') },
							{ value: 'amount_desc', label: t('scholarships.sort_amount', 'Award (highest)') }
						]}
						class="w-full"
					/>
				</div>

				<label class="flex items-center gap-2 text-sm text-muted">
					<input type="checkbox" bind:checked={filterIncludeExpired} class="accent-[var(--tool-accent)]" />
					{t('scholarships.include_expired', 'Include expired')}
				</label>

				{#if amountError}
					<p class="text-xs text-red-600" role="alert">{amountError}</p>
				{/if}
				<button
					onclick={() => applyFilters()}
					disabled={loading}
					class="w-full px-4 py-2 bg-[var(--tool-accent)] text-white rounded-lg text-sm font-medium hover:bg-[var(--tool-accent-hover)] transition-colors disabled:opacity-50"
				>
					{#if loading}
						{t('scholarships.searching', 'Searching...')}
					{:else}
						{t('scholarships.apply_filters', 'Apply filters')}
					{/if}
				</button>
			</div>
		</aside>

		<!-- Results list -->
		<div>
			{#if loading}
				<div class="space-y-3">
					<LoadingSkeleton height="120px" />
					<LoadingSkeleton height="120px" />
					<LoadingSkeleton height="120px" />
				</div>
			{:else if scholarships.length === 0}
				<EmptyState
					icon="search-x"
					title={t('scholarships.no_results', 'No scholarships found')}
					description={t('scholarships.no_results_desc', 'Try adjusting your filters or clearing them to see all scholarships.')}
				/>
			{:else}
				<div class="space-y-3">
					{#each scholarships as s (s.guide_id)}
						{@const savedItem = isSaved(s.guide_id)}
						{@const days = daysUntilDeadline(s.application_deadline)}
						<div class="p-5 bg-surface border border-border rounded-lg">
							<div class="flex items-start justify-between gap-4">
								<div class="flex-1 min-w-0">
									<a href={`/scholarships/${s.guide_id}`} class="hover:text-[var(--tool-accent)] transition-colors">
										<h3 class="text-base font-semibold text-text">{s.title}</h3>
									</a>
									{#if s.institution}
										<p class="text-sm text-text-subtle mt-0.5">{s.institution}</p>
									{/if}
								{#if s.source?.slug}
									<a
										href={mainAppUrl(`/sources/${s.source.slug}`)}
										class="inline-flex items-center gap-1 text-xs text-primary hover:underline mt-1"
									>
										{t('scholarships.source', 'Source')}: {s.source.name}
									</a>
								{/if}
									<div class="flex flex-wrap gap-2 mt-2">
										{#if s.country}
											<span class="text-xs px-2 py-0.5 bg-surface-subtle text-muted rounded">
												{s.country}
											</span>
										{/if}
										{#if s.level_of_study}
											<span class="text-xs px-2 py-0.5 bg-surface-subtle text-muted rounded capitalize">
												{s.level_of_study}
											</span>
										{/if}
										{#if s.is_renewable}
											<span class="text-xs px-2 py-0.5 bg-surface-subtle text-muted rounded">
												{t('scholarships.renewable_badge', 'Renewable')}
											</span>
										{/if}
									</div>
									{#if s.description}
										<p class="text-sm text-text-subtle mt-2 line-clamp-2">{s.description}</p>
									{/if}
								</div>
								<div class="flex-shrink-0 text-end">
									{#if s.award_amount_usd !== null && s.award_amount_usd !== undefined}
										<div class="text-lg font-bold text-[var(--tool-accent)]">
											{formatAmount(s.award_amount_usd)}
										</div>
										<div class="text-xs text-muted">{t('scholarships.award', 'award')}</div>
									{/if}
								</div>
							</div>

							<div class="flex items-center justify-between mt-4 pt-4 border-t border-border">
								<div class="flex items-center gap-3">
									{#if s.application_deadline}
										<div class="flex items-center gap-1.5 text-sm">
											<Icon name="calendar" size={14} class="text-muted" />
											<span class="text-text-subtle">{formatDate(s.application_deadline)}</span>
											{#if days !== null && days >= 0 && days <= 30}
												<span class="text-xs px-1.5 py-0.5 bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400 rounded font-medium">
													{t('scholarships.days_left', '{days} days left').replace('{days}', String(days))}
												</span>
											{:else if days !== null && days < 0}
												<span class="text-xs px-1.5 py-0.5 bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400 rounded font-medium">
													{t('scholarships.deadline_passed', 'Passed')}
												</span>
											{/if}
										</div>
									{/if}
								</div>

								<div class="flex items-center gap-2">
									{#if savedItem}
										<AutocompleteSelect
											value={savedItem.status}
											onValueChange={(v) => updateStatus(s.guide_id, v as ScholarshipStatus)}
											options={statusOptions.map((opt) => ({
												value: opt.value,
												label: t(`scholarships.status_${opt.value}`, opt.label)
											}))}
											class="text-xs"
										/>
									{/if}
									<button
										onclick={() => toggleSave(s)}
										class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium rounded-lg transition-colors {savedItem ? 'bg-[var(--tool-accent)]/10 text-[var(--tool-accent)]' : 'border border-border text-text hover:border-[var(--tool-accent)]'}"
									>
										<Icon name={savedItem ? 'bookmark-check' : 'bookmark'} size={16} />
										{savedItem ? t('scholarships.saved', 'Saved') : t('scholarships.save', 'Save')}
									</button>
									{#if s.application_url}
										<a
											href={s.application_url}
											target="_blank"
											rel="noopener noreferrer"
											class="inline-flex items-center gap-1 px-3 py-1.5 text-sm font-medium text-primary hover:text-primary-hover"
										>
											{t('scholarships.apply', 'Apply')}
											<Icon name="external-link" size={14} />
										</a>
									{/if}
								</div>
							</div>
						</div>
					{/each}
				</div>
			{/if}

			{#if totalPages > 1}
				<div class="mt-6 flex items-center justify-between gap-4 border-t border-border pt-4">
					<div class="text-sm text-muted">
						{t('scholarships.showing', 'Showing {start}–{end} of {total}')
							.replace('{start}', String(showingStart))
							.replace('{end}', String(showingEnd))
							.replace('{total}', String(total))}
					</div>
					<div class="flex items-center gap-2">
						<button
							type="button"
							disabled={currentPage <= 1}
							onclick={() => changePage(currentPage - 1)}
							class="px-3 py-1.5 text-sm rounded-lg border border-border bg-surface text-text hover:border-primary disabled:opacity-50 disabled:cursor-not-allowed"
						>
							{t('common.previous', 'Previous')}
						</button>
						<span class="text-sm text-muted">
							{t('scholarships.page_of', 'Page {page} of {totalPages}')
								.replace('{page}', String(currentPage))
								.replace('{totalPages}', String(totalPages))}
						</span>
						<button
							type="button"
							disabled={currentPage >= totalPages}
							onclick={() => changePage(currentPage + 1)}
							class="px-3 py-1.5 text-sm rounded-lg border border-border bg-surface text-text hover:border-primary disabled:opacity-50 disabled:cursor-not-allowed"
						>
							{t('common.next', 'Next')}
						</button>
					</div>
				</div>
			{/if}
		</div>
	</div>

	{#if showRegisterPrompt}
		<RegistrationPrompt
			open={showRegisterPrompt}
			onClose={() => (showRegisterPrompt = false)}
			icon="user-plus"
			title={t('register.save_scholarships', 'Save scholarships')}
			description={t('register.save_scholarships_desc', 'Create a free account to save scholarships, track your application status, and get deadline reminders.')}
		/>
	{/if}
	{/if}
</div>

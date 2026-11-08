<script lang="ts">
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import { t, i18n } from '#lib/i18n/index.js';
	import EntityHoverChip from '#lib/components/ui/EntityHoverChip.svelte';
	import { page } from '$app/state';
	import { enhance } from '$app/forms';
	import { Briefcase, Lock, ArrowRight, Plus, GitCompare, Loader2, Building2, Globe, BadgeCheck, CircleDollarSign, Users, ChevronRight, Search, Filter } from 'lucide-svelte';
	import { EmptyState, EntityHero, GuideSlideOver, LoadingSkeleton, PaginationControls, SearchInput, ListPageBar, Modal, UrlTabs, type GuideContent, type SearchSuggestion, type UrlTab } from '#lib/components/ui/index.js';
	import HeroArtJobs from '#lib/components/heroes/HeroArtJobs.svelte';
	import { formatMetric } from '#lib/components/hub/charts/format.js';
	import BookmarkButton from '#lib/components/jobs/BookmarkButton.svelte';
	import { relativeTime } from '#lib/feed/time.js';
	import { remoteImageUrl } from '#lib/utils/remoteImage.js';
	import { salaryPeriodLabel } from '#lib/utils/jobs.js';
	import { currencySymbol } from '#lib/utils/currency.js';
	import { safeHttpUrl } from '#lib/utils/safeUrl.js';
	import SelectField from '#lib/components/ui/SelectField.svelte';
	import { staticAutocompleteFetcher, autocompleteItems } from '#lib/autocomplete.js';
	import Autocomplete from '#lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '#lib/components/create/Autocomplete.svelte';
	import { api } from '#lib/api/client.js';
	import { goto } from '$app/navigation';
	import { navigating } from '$app/state';
	import type { JobListing } from '#lib/types/index.js';
	import type { PageData, ActionData } from './$types';
	import { destinationMarkets, pinMarketsFirst } from '#lib/utils/geoMarkets.js';

	let { data, form }: { data: PageData; form?: ActionData } = $props();

	const title = $derived(t('jobs.title', 'Jobs — {site}'));
	const description = $derived(
		t('jobs.description', 'Find jobs relevant to the Nigerian diaspora community.')
	);

	const jobs = $derived<JobListing[]>(data.jobs);
	const disabled = $derived(data.disabled);
	const authed = $derived(data.authed);

	let q = $state(page.url.searchParams.get('q') ?? '');
	let country = $state(page.url.searchParams.get('country') ?? '');
	let jobType = $state(page.url.searchParams.get('job_type') ?? '');
	let remote = $state(page.url.searchParams.get('remote') ?? page.url.searchParams.get('location_type') ?? '');
	let visaSponsorship = $state(page.url.searchParams.get('visa_sponsorship') === 'true');
	let sort = $state(page.url.searchParams.get('sort') ?? 'newest');
	let includeExpired = $state(page.url.searchParams.get('include_expired') === 'true');

	// A listing is expired when its application_deadline is past (the
	// backend hides these by default; the toggle opts back in).
	function isExpired(job: JobListing): boolean {
		const raw = job.field_values?.application_deadline;
		if (!raw) return false;
		const d = new Date(raw);
		if (Number.isNaN(d.getTime())) return false;
		return d.getTime() < Date.now();
	}

	// location_type is the bar's tab axis (Any location first).
	const locationTypeOptions = $derived<UrlTab[]>([
		{ value: '', label: t('jobs.any_location', 'Any location') },
		{ value: 'remote', label: t('jobs.remote', 'Remote') },
		{ value: 'hybrid', label: t('jobs.hybrid', 'Hybrid') },
		{ value: 'onsite', label: t('jobs.onsite', 'On-site') }
	]);
	const sortOptions = $derived([
		{ value: 'newest', label: t('jobs.sort_newest', 'Newest') },
		{ value: 'deadline_asc', label: t('jobs.sort_deadline', 'Deadline (soonest)') },
		{ value: 'salary_desc', label: t('jobs.sort_salary', 'Salary (highest)') }
	]);

	function applyFilters() {
		const params = new URLSearchParams();
		if (q) params.set('q', q);
		if (country) params.set('country', country);
		if (jobType) params.set('job_type', jobType);
		if (remote) params.set('remote', remote);
		if (visaSponsorship) params.set('visa_sponsorship', 'true');
		if (sort && sort !== 'newest') params.set('sort', sort);
		if (includeExpired) params.set('include_expired', 'true');
		goto(`/jobs?${params.toString()}`, { reset: false });
	}

	function clearFilters() {
		q = '';
		country = '';
		jobType = '';
		remote = '';
		visaSponsorship = false;
		sort = 'newest';
		includeExpired = false;
		goto('/jobs');
	}

	// Feed-style card body navigates to the listing; interactive children
	// (links, buttons, inputs) are excluded so their own handlers win.
	function openJob(e: MouseEvent, slug: string) {
		const el = e.target as HTMLElement | null;
		if (el?.closest('a,button,input,select,textarea,label')) return;
		goto(`/jobs/${slug}`);
	}

	function field(job: JobListing, key: string): string {
		return job.field_values?.[key] ?? '';
	}

	// Card helpers — all values come from the listing's field_values (§63
	// job_listing_fields), never hardcoded (Rule 16).
	const JOB_TYPE_BADGES: Record<string, string> = {
		full_time: 'bg-emerald-500/10 text-emerald-600',
		part_time: 'bg-amber-500/10 text-amber-600',
		contract: 'bg-blue-500/10 text-blue-600',
		freelance: 'bg-violet-500/10 text-violet-600',
		internship: 'bg-rose-500/10 text-rose-600'
	};
	function jobTypeBadge(v: string): string {
		return JOB_TYPE_BADGES[v] ?? 'bg-[var(--color-primary)]/10 text-[var(--color-primary-text)]';
	}
	function jobTypeLabel(v: string): string {
		return t(`jobs.type_${v}`, v.replace(/_/g, '-').replace(/^./, (c) => c.toUpperCase()));
	}
	function locationTypeLabel(v: string): string {
		if (v === 'remote') return t('jobs.remote', 'Remote');
		if (v === 'hybrid') return t('jobs.hybrid', 'Hybrid');
		if (v === 'onsite') return t('jobs.onsite', 'On-site');
		return v;
	}
	function fmtNum(v: string): string {
		const n = Number(v);
		return v.trim() !== '' && Number.isFinite(n) ? n.toLocaleString(i18n.locale) : v;
	}


	// Active-filter dot for the modal-held controls — q has its own dot on
	// the Search button; remote is visible in the tab row.
	const hasActiveFilters = $derived(
		!!country || !!jobType || visaSponsorship || includeExpired || (sort !== 'newest' && !!sort)
	);

	// job_type suggestions come from the seeded job_listing_fields select
	// options (public /jobs/field-options) merged with any unseeded values
	// seen on the loaded page — arbitrary input stays allowed.
	const jobTypeFetcher = $derived(
		staticAutocompleteFetcher(
			autocompleteItems([
				...(data.fieldOptions?.job_type ?? []).map((o) => o.value),
				...data.jobs.map((j) => field(j, 'job_type'))
			])
		)
	);

	// Right-side bar controls — each opens a modal rather than taking bar space.
	let filtersModalOpen = $state(false);
	let searchModalOpen = $state(false);

	const showingStart = $derived((data.currentPage - 1) * data.pageSize + 1);
	const showingEnd = $derived(Math.min(data.currentPage * data.pageSize, data.total));

	// JCE comparison state.
	let selectedJobSlugs = $state<Set<string>>(new Set());
	let savingJCE = $state(false);
	const selectedJobs = $derived<JobListing[]>(jobs.filter((j) => selectedJobSlugs.has(j.slug)));
	const canCompare = $derived(selectedJobSlugs.size >= 2 && selectedJobSlugs.size <= 4);

	function toggleJob(slug: string) {
		const next = new Set(selectedJobSlugs);
		if (next.has(slug)) {
			next.delete(slug);
		} else {
			if (next.size >= 4) return;
			next.add(slug);
		}
		selectedJobSlugs = next;
	}

	async function jobSearchFetcher(query: string): Promise<SearchSuggestion[]> {
		const res = await api.getEnvelope<JobListing[]>(`/jobs?q=${encodeURIComponent(query)}&limit=8`);
		if (res.error || !res.data) return [];
		const items = res.data ?? [];
		void res.envelope?.meta;
		return (items ?? []).map((j) => ({
			id: j.id,
			title: j.title,
			url: `/jobs/${j.slug}`,
			excerpt: j.summary
		}));
	}

	// Country filter — keyed by the canonical country name used in the job
	// custom field. Filters the full /geo/countries list client-side.
	// ADR-064 curated markets (public settings) — pinned first; every
	// seeded country stays selectable (pin-first, not restricted).
	const jobCountries = $derived(
		pinMarketsFirst(data.countries, destinationMarkets(data.siteSettings), (c) => c.geo_code)
	);

	function fetchCountries(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase().trim();
		const filtered = query === ''
			? jobCountries
			: jobCountries.filter(
					(c) => c.name.toLowerCase().includes(query) || c.geo_code.toLowerCase().includes(query)
				);
		return Promise.resolve(filtered.map((c) => ({ id: c.name, label: c.name, sublabel: c.geo_code })));
	}
	function fetchCountriesInitial(): Promise<AutocompleteItem[]> {
		return fetchCountries('');
	}
	const selectedCountryLabel = $derived(
		country ? data.countries.find((c) => c.name === country)?.name ?? country : null
	);

	// "How it works" slide-over — markdown panel + CTA repeater persisted to
	// public system settings (jobs.guide) via PUT /admin/settings/general.
	let guideOpen = $state(false);
	let guideOverride = $state<GuideContent | null>(null);
	const siteSettings = $derived((page.data.siteSettings ?? {}) as Record<string, unknown>);
	const storedGuide = $derived.by((): Partial<GuideContent> => {
		const raw = siteSettings['jobs.guide'];
		if (raw && typeof raw === 'object' && !Array.isArray(raw)) return raw as Partial<GuideContent>;
		return {};
	});
	const effectiveGuide = $derived<GuideContent>({
		title: guideOverride?.title ?? storedGuide.title ?? t('common.how_it_works', 'How it works'),
		body: guideOverride?.body ?? storedGuide.body ?? t('jobs.guide_body', ''),
		ctas: guideOverride?.ctas ?? storedGuide.ctas ?? [{ title: t('guide_panel.read_full_guide', 'Read the full guide'), url: '/guides' }]
	});

	async function saveGuide(v: GuideContent): Promise<string | null> {
		const res = await api.put('/admin/settings/general', { jobs_guide: v });
		if (res.error) return res.error.message || t('common.error', 'Something went wrong');
		guideOverride = v;
		return null;
	}
</script>

<SeoHead title={title} description={description} ogTitle={title} />

<!-- Card avatar: company logo → poster org/user avatar → initial fallback -->
{#snippet jobAvatar(src: string, label: string)}
	{#if src}
		<img
			src={remoteImageUrl(src)}
			alt={label}
			class="h-10 w-10 shrink-0 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] object-contain"
		/>
	{:else}
		<div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl border border-[var(--color-border)] bg-[var(--color-surface-subtle)] text-base font-semibold text-[var(--color-muted)]">
			{label.charAt(0).toUpperCase()}
		</div>
	{/if}
{/snippet}

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 48rem)/2))">
	<!-- Header — EntityHero directory panel -->
	<EntityHero
		class="mt-4"
		icon={Briefcase}
		accent="var(--color-primary)"
		heading={t('jobs.heading', 'Jobs')}
		description={t('jobs.hero_desc', 'Find diaspora-friendly roles — remote work, visa sponsorship, and opportunities back home.')}
		crumbs={[
			{ label: t('common.home', 'Home'), href: '/' },
			{ label: t('jobs.heading', 'Jobs') }
		]}
		stats={[
			{ icon: Briefcase, value: formatMetric(data.total), label: t('jobs.stats_jobs', 'Open roles') },
			{ icon: Globe, value: formatMetric(data.countries.length), label: t('jobs.stats_countries', 'Countries') }
		]}
	>
		{#snippet art()}
			<HeroArtJobs accent="var(--color-primary)" />
		{/snippet}
		{#snippet actions(c)}
			<div class="grid w-full grid-cols-2 gap-3 sm:flex sm:items-center">
				<a href="#jobs-results" class="{c.primary} flex-1 justify-center whitespace-nowrap">
					<Search size={15} /> {t('jobs.find', 'Browse jobs')}
				</a>
				{#if authed && !disabled}
					<a href="/jobs/create" class="{c.secondary} flex-1 justify-center whitespace-nowrap">
						<Plus size={15} /> {t('jobs.create_button', 'Post a job')}
					</a>
					<a href="/me/applications" class="{c.ghost} flex-1 justify-center whitespace-nowrap">
						{t('jobs.my_applications_link', 'My applications')}
						<ChevronRight size={14} class="rtl:-scale-x-100" />
					</a>
				{/if}
				<button type="button" onclick={() => (guideOpen = true)} class="{c.ghost} flex-1 justify-center whitespace-nowrap {authed && !disabled ? '' : 'col-span-2 sm:col-span-1'}">
					{t('common.how_it_works', 'How it works')}
					<ChevronRight size={14} class="rtl:-scale-x-100" />
				</button>
			</div>
		{/snippet}
	</EntityHero>

	{#if !disabled}
		<!-- Location tabs on the left; Filters + Search buttons on the right —
		     one ListPageBar row (the /org, /users pattern). -->
		<ListPageBar>
			<UrlTabs
				tabs={locationTypeOptions}
				value={remote}
				onValueChange={(v) => { remote = v; applyFilters(); }}
				class="!border-b-0 !bg-transparent"
			/>
			{#snippet actions()}
				<button
					type="button"
					onclick={() => (filtersModalOpen = true)}
					class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					{t('common.filters', 'Filters')}
					{#if hasActiveFilters}
						<span class="ms-0.5 h-2 w-2 rounded-full bg-[var(--color-primary)]" aria-hidden="true"></span>
					{/if}
					<Filter size={16} />
				</button>
				<button
					type="button"
					onclick={() => (searchModalOpen = true)}
					class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					{t('common.search', 'Search')}
					{#if q.trim()}
						<span class="ms-0.5 h-2 w-2 rounded-full bg-[var(--color-primary)]" aria-hidden="true"></span>
					{/if}
					<Search size={16} />
				</button>
			{/snippet}
		</ListPageBar>

		<!-- Filters modal — the remaining controls (location_type lives in
		     the tab row now). Each control applies on commit. -->
		<Modal
			bind:open={filtersModalOpen}
			title={t('common.filters', 'Filters')}
			icon={Filter}
			size="lg"
		>
			{#snippet children()}
				<div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
					<Autocomplete
						placeholder={t('jobs.country', 'Country')}
						ariaLabel={t('jobs.country', 'Country')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchCountries}
						initialFetcher={fetchCountriesInitial}
						selectedId={country || null}
						selectedLabel={selectedCountryLabel}
						onSelect={(item) => { country = item.id; applyFilters(); }}
						onClear={() => { country = ''; applyFilters(); }}
					/>
					<Autocomplete
						placeholder={t('jobs.job_type', 'Job type')}
						ariaLabel={t('jobs.job_type', 'Job type')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={jobTypeFetcher}
						initialFetcher={jobTypeFetcher}
						selectedId={jobType || null}
						selectedLabel={jobType || null}
						allowFreeText
						onSelect={(item) => { jobType = item.id; applyFilters(); }}
						onClear={() => { jobType = ''; applyFilters(); }}
					/>
					<SelectField
						value={sort}
						options={sortOptions}
						placeholder={t('jobs.sort_label', 'Sort')}
						ariaLabel={t('jobs.sort_label', 'Sort')}
						onValueChange={(v) => { sort = v; applyFilters(); }}
						class="w-full"
					/>
					<div class="flex flex-wrap items-center gap-x-4 gap-y-2">
						<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
							<input
								type="checkbox"
								checked={visaSponsorship}
								onchange={() => { visaSponsorship = !visaSponsorship; applyFilters(); }}
								class="accent-[var(--color-primary)]"
							/>
							{t('jobs.visa_sponsorship', 'Visa sponsorship')}
						</label>
						<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
							<input
								type="checkbox"
								checked={includeExpired}
								onchange={() => { includeExpired = !includeExpired; applyFilters(); }}
								class="accent-[var(--color-primary)]"
							/>
							{t('jobs.include_expired', 'Include expired')}
						</label>
					</div>
				</div>
				{#if hasActiveFilters}
					<button
						type="button"
						onclick={clearFilters}
						class="mt-3 text-xs text-[var(--color-muted)] underline hover:text-[var(--color-text)]"
					>
						{t('common.clear_filters', 'Clear filters')}
					</button>
				{/if}
			{/snippet}
		</Modal>

		<!-- Search modal — same GET form as the previous panel search. -->
		<Modal bind:open={searchModalOpen} title={t('common.search', 'Search')} icon={Search} size="sm">
			{#snippet children()}
				<form action="/jobs" method="get" role="search" onsubmit={() => (searchModalOpen = false)}>
					<SearchInput
						bind:value={q}
						placeholder={t('jobs.search', 'Search jobs…')}
						ariaLabel={t('jobs.heading', 'Jobs')}
						fetcher={jobSearchFetcher}
					/>
					<!-- Hidden fields to preserve the active filters on submit -->
					{#if country}
						<input type="hidden" name="country" value={country} />
					{/if}
					{#if jobType}
						<input type="hidden" name="job_type" value={jobType} />
					{/if}
					{#if remote}
						<input type="hidden" name="remote" value={remote} />
					{/if}
					{#if visaSponsorship}
						<input type="hidden" name="visa_sponsorship" value="true" />
					{/if}
					{#if sort && sort !== 'newest'}
						<input type="hidden" name="sort" value={sort} />
					{/if}
					{#if includeExpired}
						<input type="hidden" name="include_expired" value="true" />
					{/if}
				</form>
			{/snippet}
		</Modal>
	{/if}

	{#if disabled}
		<div class="mx-auto w-full max-w-3xl p-4">
			<EmptyState
				icon={Lock}
				title={t('tools.disabled_title', 'This tool is currently unavailable')}
				body={t('tools.disabled_body', 'Check back soon or contact support.')}
			/>
		</div>
	{:else}

		<!-- Results -->
		<div id="jobs-results" class="mx-auto w-full max-w-3xl p-4">
			{#if navigating.to}
				<LoadingSkeleton variant="card" />
			{:else if jobs.length === 0}
				<EmptyState
					icon={Briefcase}
					title={t('jobs.empty', 'No jobs found')}
					body={t('jobs.empty_body', 'Try adjusting your filters.')}
				/>
			{:else}
				{#if data.total > 0}
					<p class="mb-3 text-xs text-[var(--color-muted)]">
						{t('common.showing', 'Showing {start}-{end} of {total}').replace('{start}', String(showingStart)).replace('{end}', String(showingEnd)).replace('{total}', String(data.total))}
					</p>
				{/if}

				{#if form?.error}
					<div class="mb-4 rounded-[var(--radius)] border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300">
						{form.message || t('jobs.jce_error', 'Could not create comparison')}
					</div>
				{/if}

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
					{#if selectedJobSlugs.size > 0}
						<div class="mb-4 flex items-center justify-between flex-wrap gap-3">
							<p class="text-sm text-[var(--color-muted)]">
								{t('jobs.selected', 'Selected')}: {selectedJobSlugs.size} / 4
							</p>
							{#each selectedJobs as job (job.slug)}
								<input type="hidden" name="job_slug" value={job.slug} />
							{/each}
							<button
								type="submit"
								disabled={!canCompare || !authed}
								title={authed
									? t('jobs.compare_in_jce_hint', 'Open in JCE comparison')
									: t('jobs.sign_in_to_compare', 'Sign in to compare')}
								class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed"
							>
								{#if savingJCE}
									<Loader2 size={16} class="animate-spin" />
								{:else}
									<GitCompare size={16} />
								{/if}
								{t('jobs.compare_in_jce', 'Compare in JCE')}
							</button>
						</div>
					{/if}

					<div class="flex flex-col gap-3">
						{#each jobs as job (job.id)}
							{@const jt = field(job, 'job_type')}
							{@const lt = field(job, 'location_type')}
							{@const poster = job.poster}
							{@const source = job.source}
							{@const company = field(job, 'company_name') || poster?.name || source?.name || ''}
							{@const logo = field(job, 'company_logo') || poster?.logo_url || source?.logo_url || ''}
							{@const extHref = source ? safeHttpUrl(field(job, 'company_website') || source.source_url) : ''}
							{@const loc = [field(job, 'city'), field(job, 'country')].filter(Boolean).join(', ')}
							{@const salMin = field(job, 'salary_min')}
							{@const salMax = field(job, 'salary_max')}
							{@const salCur = field(job, 'salary_currency')}
							{@const salPeriod = salaryPeriodLabel(field(job, 'salary_period'))}
							{@const applicants = job.applicant_count}
							{@const posted = relativeTime(job.published_at, new Date(), i18n.locale)}
							{@const secondary = [lt === 'remote' ? t('jobs.remote', 'Remote') : loc, posted]
								.filter(Boolean)
								.join(' · ')}
							<div
								role="link"
								tabindex="0"
								onclick={(e) => openJob(e, job.slug)}
								onkeydown={(e) => {
									if (e.key === 'Enter') {
										e.preventDefault();
										goto(`/jobs/${job.slug}`);
									}
								}}
								class="group relative flex cursor-pointer flex-col gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:bg-[color-mix(in_srgb,var(--color-surface-subtle)_40%,transparent)]"
							>
								<!-- Identity row: logo + company/poster name (hover card → profile or external site) + compare/bookmark -->
								<div class="flex items-center gap-2">
									{#if source?.id}
										<EntityHoverChip
											type="source"
											id={source.id}
											slug={source.slug}
											name={company || source.name}
											avatar={logo}
											externalHref={extHref}
											class="flex min-w-0 items-center gap-3 text-start"
										>
											{#snippet trigger()}
												{@render jobAvatar(logo, company || source.name)}
												<span class="min-w-0">
													<span class="flex items-center gap-1 text-sm font-semibold text-[var(--color-text)]">
														<span class="truncate">{company || source.name}</span>
														<BadgeCheck size={14} class="shrink-0 text-[var(--color-primary-text)]" />
													</span>
													{#if secondary}
														<span class="block truncate text-xs text-[var(--color-muted)]">{secondary}</span>
													{/if}
												</span>
											{/snippet}
										</EntityHoverChip>
									{:else if poster}
										<EntityHoverChip
											type={poster.kind}
											id={poster.id}
											slug={poster.slug}
											name={company || poster.name}
											avatar={logo}
											class="flex min-w-0 items-center gap-3 text-start"
										>
											{#snippet trigger()}
												{@render jobAvatar(logo, company || poster.name)}
												<span class="min-w-0">
													<span class="flex items-center gap-1 text-sm font-semibold text-[var(--color-text)]">
														<span class="truncate">{company || poster.name}</span>
														{#if poster.verified}<BadgeCheck size={14} class="shrink-0 text-[var(--color-primary-text)]" />{/if}
													</span>
													{#if secondary}
														<span class="block truncate text-xs text-[var(--color-muted)]">{secondary}</span>
													{/if}
												</span>
											{/snippet}
										</EntityHoverChip>
									{:else}
										<div class="flex min-w-0 items-center gap-3">
											{@render jobAvatar(logo, company || job.title)}
											<span class="min-w-0">
												{#if company}
													<span class="block truncate text-sm font-semibold text-[var(--color-text)]">{company}</span>
												{/if}
												{#if secondary}
													<span class="block truncate text-xs text-[var(--color-muted)]">{secondary}</span>
												{/if}
											</span>
										</div>
									{/if}
									<div class="ms-auto flex shrink-0 items-center gap-1">
										<input
											type="checkbox"
											checked={selectedJobSlugs.has(job.slug)}
											onchange={() => toggleJob(job.slug)}
											disabled={selectedJobSlugs.size >= 4 && !selectedJobSlugs.has(job.slug)}
											title={t('jobs.compare_in_jce_hint', 'Select up to 4 jobs to compare side by side.')}
											aria-label={t('jobs.compare_in_jce_hint', 'Select up to 4 jobs to compare side by side.')}
											class="accent-[var(--color-primary)]"
										/>
										<!-- job_listing rows live in contents, so §14 content bookmarks apply -->
										<BookmarkButton entityType="content" entityId={job.id} bookmarked={false} />
									</div>
								</div>

								<!-- Title + type badge -->
								<div class="flex flex-wrap items-center gap-x-2.5 gap-y-1.5">
									<a href="/jobs/{job.slug}" class="min-w-0">
										<h3 class="text-[17px] font-semibold leading-snug tracking-tight text-[var(--color-text)] transition-colors group-hover:text-[var(--color-primary-text)]">
											{job.title}
										</h3>
									</a>
									{#if jt}
										<span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-[11px] font-semibold uppercase tracking-wide {jobTypeBadge(jt)}">
											{jobTypeLabel(jt)}
										</span>
									{/if}
									{#if isExpired(job)}
										<span class="inline-flex items-center rounded-full bg-[var(--color-danger)]/10 px-2 py-0.5 text-[10px] font-medium text-[var(--color-danger)]">
											{t('jobs.expired', 'Expired')}
										</span>
									{/if}
									{#if field(job, 'is_featured') === 'true'}
										<span class="inline-flex items-center rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-[10px] font-medium text-[var(--color-primary-text)]">
											{t('jobs.featured', 'Featured')}
										</span>
									{/if}
								</div>

								{#if job.summary}
									<p class="line-clamp-2 text-sm text-[var(--color-text-subtle)]">{job.summary}</p>
								{/if}

								<!-- Meta -->
								{#if lt || salMin || salMax || applicants}
									<div class="flex flex-wrap items-center gap-x-4 gap-y-1.5 text-sm text-[var(--color-muted)]">
										{#if lt}
											<span class="flex items-center gap-1.5">
												{#if lt === 'remote'}
													<Globe size={14} class="shrink-0" />
													<span>{field(job, 'country') || t('jobs.worldwide', 'Worldwide')}</span>
												{:else}
													<Building2 size={14} class="shrink-0" />
													<span>{locationTypeLabel(lt)}</span>
												{/if}
											</span>
										{/if}
										{#if salMin || salMax}
											<span class="flex items-center gap-1.5">
												<CircleDollarSign size={14} class="shrink-0 text-emerald-500" />
												<span class="font-semibold text-[var(--color-text)]">
													{#if salCur}{@const sym = currencySymbol(salCur, data.currencies)}{sym === salCur.trim().toUpperCase() ? `${sym} ` : sym}{/if}{[fmtNum(salMin), fmtNum(salMax)].filter(Boolean).join(' – ')}
												</span>
												{#if salPeriod}
													<span class="text-xs">{salPeriod}</span>
												{/if}
											</span>
										{/if}
										{#if applicants}
											<span class="flex items-center gap-1.5">
												<Users size={14} class="shrink-0" />
												{applicants === 1 ? t('jobs.applicant_one', '1 applicant') : t('jobs.applicants', '{count} applicants', { count: applicants })}
											</span>
										{/if}
									</div>
								{/if}

								<!-- Actions -->
								<div class="flex items-center justify-between gap-3">
									<a
										href="/jobs/{job.slug}"
										class="inline-flex items-center gap-1 text-sm font-medium text-[var(--color-primary-text)] hover:underline"
									>
										{t('jobs.view_details', 'View Details')}
										<ChevronRight class="rtl:-scale-x-100" size={15} />
									</a>
									{#if safeHttpUrl(field(job, 'application_url'))}
										<a
											href={safeHttpUrl(field(job, 'application_url'))}
											target="_blank"
											rel="noopener noreferrer"
											class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)]/10 px-4 py-2 text-sm font-medium text-[var(--color-primary-text)] transition-colors hover:bg-[var(--color-primary)]/20"
										>
											{t('jobs.quick_apply', 'Quick Apply')}
											<ArrowRight class="rtl:-scale-x-100" size={15} />
										</a>
									{:else}
										<a
											href="/jobs/{job.slug}#apply"
											class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)]/10 px-4 py-2 text-sm font-medium text-[var(--color-primary-text)] transition-colors hover:bg-[var(--color-primary)]/20"
										>
											{t('jobs.quick_apply', 'Quick Apply')}
											<ArrowRight class="rtl:-scale-x-100" size={15} />
										</a>
									{/if}
								</div>
							</div>
						{/each}
					</div>
				</form>

				{#if data.total > data.pageSize}
					<div class="mt-4">
						<PaginationControls
							page={data.currentPage}
							totalPages={Math.ceil(data.total / data.pageSize)}
							onPageChange={(p) => {
						const params = new URLSearchParams(page.url.search);
						params.set('page', String(p));
						goto(`/jobs?${params.toString()}`);
					}}
						/>
					</div>
				{/if}
			{/if}
		</div>
	{/if}
</div>

<!-- "How it works" slide-over — GuideSlideOver; admins edit inline (jobs.guide). -->
<GuideSlideOver
	bind:open={guideOpen}
	title={effectiveGuide.title}
	body={effectiveGuide.body}
	ctas={effectiveGuide.ctas}
	canManage={data.canManage}
	onSave={saveGuide}
/>

<script lang="ts">
	import { t, i18n } from '$lib/i18n';
	import EntityHoverChip from '$lib/components/ui/EntityHoverChip.svelte';
	import { page } from '$app/state';
	import { enhance } from '$app/forms';
	import { Briefcase, Lock, ArrowRight, Plus, GitCompare, Loader2, Building2, Globe, BadgeCheck, CircleDollarSign, Users, ChevronRight } from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton, PaginationControls, SearchInput, ListPageBar, FilterToggle, PageAction, type SearchSuggestion } from '$lib/components/ui';
	import BookmarkButton from '$lib/components/jobs/BookmarkButton.svelte';
	import { relativeTime } from '$lib/feed/time';
	import { remoteImageUrl } from '$lib/utils/remoteImage';
	import { salaryPeriodLabel } from '$lib/utils/jobs';
	import { currencySymbol } from '$lib/utils/currency';
	import { safeHttpUrl } from '$lib/utils/safeUrl';
	import SelectField from '$lib/components/ui/SelectField.svelte';
	import { staticAutocompleteFetcher, autocompleteItems } from '$lib/autocomplete';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import { navigating } from '$app/state';
	import type { JobListing } from '$lib/types';
	import type { PageData, ActionData } from './$types';

	let { data, form }: { data: PageData; form?: ActionData } = $props();

	const title = $derived(t('jobs.title', 'Jobs — JapaConnect'));
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

	const locationTypeOptions = $derived([
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
		goto(`/jobs?${params.toString()}`, { keepFocus: true, noScroll: true });
	}

	// Debounced auto-apply for the free-text search field.
	let qTimer: ReturnType<typeof setTimeout> | null = null;
	function queueApply() {
		if (qTimer) clearTimeout(qTimer);
		qTimer = setTimeout(applyFilters, 400);
	}

	function clearFilters() {
		q = '';
		country = '';
		jobType = '';
		remote = '';
		visaSponsorship = false;
		sort = 'newest';
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
		return JOB_TYPE_BADGES[v] ?? 'bg-[var(--color-primary)]/10 text-[var(--color-primary)]';
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


	const hasActiveFilters = $derived(
		!!q || !!country || !!jobType || !!remote || visaSponsorship || (sort !== 'newest' && !!sort)
	);

	// job_type is a free-text filter over listing field_values — suggest the
	// values present in the loaded page, allow arbitrary input via allowFreeText.
	const jobTypeFetcher = $derived(
		staticAutocompleteFetcher(autocompleteItems(data.jobs.map((j) => field(j, 'job_type'))))
	);

	let showFilters = $state(false);
	$effect(() => {
		if (hasActiveFilters) showFilters = true;
	});
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
		const res = await api.get<JobListing[]>(`/jobs?q=${encodeURIComponent(query)}&limit=8`);
		if (res.error || !res.data) return [];
		return (res.data ?? []).map((j) => ({
			id: j.id,
			title: j.title,
			url: `/jobs/${j.slug}`,
			excerpt: j.summary
		}));
	}

	// Country filter — keyed by the canonical country name used in the job
	// custom field. Filters the full /geo/countries list client-side.
	function fetchCountries(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase().trim();
		const filtered = query === ''
			? data.countries
			: data.countries.filter(
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
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

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

<div class="mx-auto w-full max-w-4xl" style="--page-gutter: 1rem">
	<ListPageBar title={t('jobs.heading', 'Jobs')}>
		{#snippet actions()}
			{#if !disabled}
				<FilterToggle bind:show={showFilters} active={hasActiveFilters} controls="jobs-filter-panel" />
			{/if}
			{#if authed && !disabled}
				<PageAction
					href="/me/applications"
					icon={Briefcase}
					label={t('jobs.my_applications_link', 'My applications')}
				/>
				<PageAction
					href="/jobs/create"
					icon={Plus}
					label={t('jobs.create_button', 'Post a job')}
				/>
			{/if}
		{/snippet}
	</ListPageBar>

	{#if disabled}
		<div class="p-4">
			<EmptyState
				icon={Lock}
				title={t('tools.disabled_title', 'This tool is currently unavailable')}
				body={t('tools.disabled_body', 'Check back soon or contact support.')}
			/>
		</div>
	{:else}
		<!-- Filter panel -->
		{#if showFilters}
		<div id="jobs-filter-panel" class="border-b border-[var(--color-border)] px-4 py-3" style="margin-inline: var(--page-gutter)">
			<form onsubmit={(e) => { e.preventDefault(); applyFilters(); }} class="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
				<SearchInput
					bind:value={q}
					placeholder={t('jobs.search', 'Search jobs…')}
					ariaLabel={t('jobs.heading', 'Jobs')}
					fetcher={jobSearchFetcher}
					onInput={queueApply}
					inputClass="focus:ring-2 focus:ring-[var(--color-primary)]"
				/>
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
					value={remote}
					options={locationTypeOptions}
					onValueChange={(v) => { remote = v; applyFilters(); }}
				/>
				<SelectField
					value={sort}
					options={sortOptions}
					onValueChange={(v) => { sort = v; applyFilters(); }}
				/>
				<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
					<input
						type="checkbox"
						checked={visaSponsorship}
						onchange={() => { visaSponsorship = !visaSponsorship; applyFilters(); }}
						class="accent-[var(--color-primary)]"
					/>
					{t('jobs.visa_sponsorship', 'Visa sponsorship')}
				</label>
			</form>
			{#if hasActiveFilters}
				<button
					type="button"
					onclick={clearFilters}
					class="mt-2 text-xs text-[var(--color-muted)] underline hover:text-[var(--color-text)]"
				>
					{t('common.clear_filters', 'Clear filters')}
				</button>
			{/if}
		</div>
		{/if}

		<!-- Results -->
		<div class="p-4">
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
							{@const applicants = field(job, 'applicant_count')}
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
											class="flex min-w-0 items-center gap-3 text-left"
										>
											{#snippet trigger()}
												{@render jobAvatar(logo, company || source.name)}
												<span class="min-w-0">
													<span class="flex items-center gap-1 text-sm font-semibold text-[var(--color-text)]">
														<span class="truncate">{company || source.name}</span>
														<BadgeCheck size={14} class="shrink-0 text-[var(--color-primary)]" />
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
											class="flex min-w-0 items-center gap-3 text-left"
										>
											{#snippet trigger()}
												{@render jobAvatar(logo, company || poster.name)}
												<span class="min-w-0">
													<span class="flex items-center gap-1 text-sm font-semibold text-[var(--color-text)]">
														<span class="truncate">{company || poster.name}</span>
														{#if poster.verified}<BadgeCheck size={14} class="shrink-0 text-[var(--color-primary)]" />{/if}
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
									<div class="ml-auto flex shrink-0 items-center gap-1">
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
										<h3 class="text-[17px] font-semibold leading-snug tracking-tight text-[var(--color-text)] transition-colors group-hover:text-[var(--color-primary)]">
											{job.title}
										</h3>
									</a>
									{#if jt}
										<span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-[11px] font-semibold uppercase tracking-wide {jobTypeBadge(jt)}">
											{jobTypeLabel(jt)}
										</span>
									{/if}
									{#if field(job, 'is_featured') === 'true'}
										<span class="inline-flex items-center rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-[10px] font-medium text-[var(--color-primary)]">
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
												{t('jobs.applicants', '{count} applicants', { count: applicants })}
											</span>
										{/if}
									</div>
								{/if}

								<!-- Actions -->
								<div class="flex items-center justify-between gap-3">
									<a
										href="/jobs/{job.slug}"
										class="inline-flex items-center gap-1 text-sm font-medium text-[var(--color-primary)] hover:underline"
									>
										{t('jobs.view_details', 'View Details')}
										<ChevronRight size={15} />
									</a>
									{#if safeHttpUrl(field(job, 'application_url'))}
										<a
											href={safeHttpUrl(field(job, 'application_url'))}
											target="_blank"
											rel="noopener noreferrer"
											class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)]/10 px-4 py-2 text-sm font-medium text-[var(--color-primary)] transition-colors hover:bg-[var(--color-primary)]/20"
										>
											{t('jobs.quick_apply', 'Quick Apply')}
											<ArrowRight size={15} />
										</a>
									{:else}
										<a
											href="/jobs/{job.slug}#apply"
											class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)]/10 px-4 py-2 text-sm font-medium text-[var(--color-primary)] transition-colors hover:bg-[var(--color-primary)]/20"
										>
											{t('jobs.quick_apply', 'Quick Apply')}
											<ArrowRight size={15} />
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
						const params = new URLSearchParams(page.url.searchParams);
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

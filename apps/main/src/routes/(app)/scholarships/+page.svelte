<script lang="ts">
	import { t } from '$lib/i18n';
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { GraduationCap, Lock, Bookmark, BookmarkCheck, Search, ClipboardList } from 'lucide-svelte';
	import { EmptyState, PaginationControls, SearchInput, type SearchSuggestion } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import type { Scholarship } from '$lib/types';
	import type { PageData } from './$types';

	// SavedScholarship is exported from +page.server.ts as
	// SavedScholarshipView and inferred by $types via PageData, but we
	// redeclare the interface here for direct use in component state
	// typing (SvelteKit's $types does not re-export non-inferred types).
	// This is the flat projection of the backend's nested SavedScholarship
	// response (id + guide_id + title + status + notes).
	interface SavedScholarship {
		id: string;
		guide_id: string;
		title: string;
		slug: string;
		summary?: string;
		status: string;
		notes?: string;
		remind_before_days?: number | null;
		updated_at?: string;
	}

	let { data }: { data: PageData } = $props();

	const title = $derived(t('tools.scholarships', 'Scholarship Finder — JapaConnect'));
	const description = $derived(
		t('tools.scholarships_desc', 'Find and track scholarships by country and level.')
	);

	const scholarships = $derived<Scholarship[]>(data.scholarships);
	const disabled = $derived(data.disabled);
	const authed = $derived(data.authed);
	let savedScholarships = $state<SavedScholarship[]>(untrack(() => data.savedScholarships ?? []));

	// Filter state — initialized from URL.
	let q = $state(page.url.searchParams.get('q') ?? '');
	let country = $state(page.url.searchParams.get('country') ?? '');
	let level = $state(page.url.searchParams.get('level') ?? '');
	let savedIds = $state<Set<string>>(untrack(() => new Set(data.savedIds ?? [])));

	// Application status options — driven by the backend's saved-scholarship
	// PATCH endpoint (Phase 33.04 application tracker). The values must
	// match the backend CHECK constraint + IsValidStatus exactly:
	// saved/researching/applied/accepted/rejected/withdrawn.
	const statusOptions = $derived([
		{ value: 'saved', label: t('scholarships.status_saved', 'Saved') },
		{ value: 'researching', label: t('scholarships.status_researching', 'Researching') },
		{ value: 'applied', label: t('scholarships.status_applied', 'Applied') },
		{ value: 'accepted', label: t('scholarships.status_accepted', 'Accepted') },
		{ value: 'rejected', label: t('scholarships.status_rejected', 'Rejected') },
		{ value: 'withdrawn', label: t('scholarships.status_withdrawn', 'Withdrawn') }
	]);

	const statusBadgeColor = (status: string): string => {
		if (status === 'accepted') return 'bg-[var(--color-success)] text-white';
		if (status === 'rejected' || status === 'withdrawn') return 'bg-[var(--color-danger)] text-white';
		if (status === 'applied' || status === 'researching') return 'bg-[var(--color-info)] text-white';
		return 'bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]';
	};

	function applyFilters() {
		const params = new URLSearchParams();
		if (q) params.set('q', q);
		if (country) params.set('country', country);
		if (level) params.set('level', level);
		goto(`/scholarships?${params.toString()}`, { keepFocus: true });
	}

	function clearFilters() {
		q = '';
		country = '';
		level = '';
		goto('/scholarships');
	}

	const hasActiveFilters = $derived(!!q || !!country || !!level);
	// 1-based page (data.currentPage is 1-based); showing range is
	// [(page-1)*size + 1, min(page*size, total)].
	const showingStart = $derived((data.currentPage - 1) * data.pageSize + 1);
	const showingEnd = $derived(Math.min(data.currentPage * data.pageSize, data.total));

	async function toggleSave(s: Scholarship) {
		if (!authed) return;
		if (savedIds.has(s.guide_id)) {
			const res = await api.delete(`/scholarships/${s.guide_id}/save`);
			if (res.error) return;
			savedIds.delete(s.guide_id);
			savedIds = new Set(savedIds);
			// Keep the Application Tracker list in sync with savedIds so
			// unsaving removes the entry immediately without a reload.
			savedScholarships = savedScholarships.filter((x) => x.guide_id !== s.guide_id);
		} else {
			const res = await api.post(`/scholarships/${s.guide_id}/save`);
			if (res.error) return;
			savedIds.add(s.guide_id);
			savedIds = new Set(savedIds);
			// Add a new tracker entry so the Application Tracker reflects
			// the save immediately. Status defaults to 'saved' (backend
			// default); the user can change it via the tracker select.
			savedScholarships = [
				...savedScholarships,
				{
					id: s.guide_id,
					guide_id: s.guide_id,
					title: s.title,
					slug: s.slug,
					summary: s.summary,
					status: 'saved',
					notes: '',
					remind_before_days: null,
					updated_at: new Date().toISOString()
				}
			];
		}
	}

	// Phase 33.04 — update the application status of a saved scholarship.
	// Only mutates local state after the server confirms success; on error
	// the select reverts to the previous value via Svelte's reactivity.
	async function updateStatus(saved: SavedScholarship, status: string) {
		const prev = saved.status;
		const res = await api.patch(`/scholarships/${saved.guide_id}/save`, { status });
		if (res.error) {
			// Revert: re-assign the object so Svelte re-renders the select.
			savedScholarships.splice(savedScholarships.indexOf(saved), 1, { ...saved, status: prev });
			return;
		}
		savedScholarships.splice(savedScholarships.indexOf(saved), 1, { ...saved, status });
	}

	function field(s: Scholarship, key: string): string {
		return s.field_values?.[key] ?? '';
	}

	async function scholarshipSearchFetcher(q: string): Promise<SearchSuggestion[]> {
		const res = await api.get<Scholarship[]>(`/scholarships?q=${encodeURIComponent(q)}&limit=8`);
		if (res.error || !res.data) return [];
		// The backend returns the scholarship array directly in `data`
		// (`{ data: [...], meta: {...} }`), not wrapped in a `scholarships`
		// key — reading `res.data.scholarships` here made suggestions empty.
		return (res.data ?? []).map((s) => ({
			id: s.guide_id,
			title: s.title,
			url: `/scholarships?q=${encodeURIComponent(s.slug)}`,
			excerpt: s.summary
		}));
	}

	// JSON-LD ItemList of scholarships for SEO (Ref: Section 73, ROADMAP-MAIN 36.03).
	const jsonLd = $derived(
		scholarships.length > 0
			? JSON.stringify({
					'@context': 'https://schema.org',
					'@type': 'ItemList',
					name: t('tools.scholarships', 'Scholarship Finder'),
					itemListElement: scholarships.map((s, i) => ({
						'@type': 'ListItem',
						position: i + 1,
						item: {
							'@type': 'EducationalOccupationalProgram',
							name: s.title,
							description: s.summary,
							url: `${page.url.origin}/scholarships?q=${encodeURIComponent(s.slug)}`
						}
					}))
				})
			: ''
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	{#if jsonLd}
		{@html `<script type="application/ld+json">${jsonLd}</script>`}
	{/if}
</svelte:head>

<div class="mx-auto w-full max-w-4xl">
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<GraduationCap size={20} /> {t('tools.scholarships', 'Scholarship Finder')}
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
	{:else}
		<!-- Filter panel -->
		<div class="border-b border-[var(--color-border)] px-4 py-3">
			<form onsubmit={(e) => { e.preventDefault(); applyFilters(); }} class="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
				<SearchInput
					bind:value={q}
					placeholder={t('scholarships.search', 'Search scholarships…')}
					ariaLabel={t('tools.scholarships', 'Scholarship Finder')}
					fetcher={scholarshipSearchFetcher}
					inputClass="focus:ring-2 focus:ring-[var(--color-primary)]"
				/>
				<input
					type="text"
					placeholder={t('scholarships.country', 'Country')}
					bind:value={country}
					class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
				/>
				<input
					type="text"
					placeholder={t('scholarships.level', 'Level')}
					bind:value={level}
					class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
				/>
				<button
					type="submit"
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-90"
				>
					{t('common.filter', 'Filter')}
				</button>
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

		<!-- Application tracker — Phase 33.04 -->
		{#if authed && savedScholarships.length > 0}
			<div class="border-b border-[var(--color-border)] px-4 py-3">
				<h2 class="mb-3 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
					<ClipboardList size={16} />
					{t('scholarships.tracker', 'Application Tracker')}
				</h2>
				<div class="space-y-2">
					{#each savedScholarships as saved (saved.guide_id)}
						<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
							<div class="flex items-start justify-between gap-3">
								<div class="min-w-0 flex-1">
									<h3 class="truncate text-sm font-medium text-[var(--color-text)]">{saved.title}</h3>
									<div class="mt-1 flex items-center gap-2">
										<span class="rounded-full px-2 py-0.5 text-xs font-medium {statusBadgeColor(saved.status)}">
											{statusOptions.find((o) => o.value === saved.status)?.label ?? saved.status}
										</span>
										{#if saved.updated_at}
											<span class="text-xs text-[var(--color-muted)]">{t('scholarships.updated', 'Updated')}: {saved.updated_at}</span>
										{/if}
									</div>
								</div>
								<select
									value={saved.status}
									onchange={(e) => updateStatus(saved, (e.target as HTMLSelectElement).value)}
									class="shrink-0 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-xs text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
									aria-label={t('scholarships.status_label', 'Application status')}
								>
									{#each statusOptions as opt (opt.value)}
										<option value={opt.value}>{opt.label}</option>
									{/each}
								</select>
							</div>
						</div>
					{/each}
				</div>
			</div>
		{/if}

		<!-- Results -->
		<div class="p-4">
			{#if scholarships.length === 0}
				<EmptyState
					icon={GraduationCap}
					title={t('scholarships.empty', 'No scholarships found')}
					body={t('scholarships.empty_body', 'Try adjusting your filters.')}
				/>
			{:else}
				{#if data.total > 0}
					<p class="mb-3 text-xs text-[var(--color-muted)]">
						{t('common.showing', 'Showing {start}-{end} of {total}').replace('{start}', String(showingStart)).replace('{end}', String(showingEnd)).replace('{total}', String(data.total))}
					</p>
				{/if}
				<div class="space-y-3">
					{#each scholarships as s (s.guide_id)}
						<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
							<div class="flex items-start justify-between gap-3">
								<div class="min-w-0 flex-1">
									<h3 class="text-sm font-medium text-[var(--color-text)]">{s.title}</h3>
									{#if s.summary}
										<p class="mt-1 line-clamp-2 text-xs text-[var(--color-muted)]">{s.summary}</p>
									{/if}
									<div class="mt-2 flex flex-wrap gap-2 text-xs text-[var(--color-muted)]">
										{#if field(s, 'countries_eligible')}
											<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">{field(s, 'countries_eligible')}</span>
										{/if}
										{#if field(s, 'level_of_study')}
											<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">{field(s, 'level_of_study')}</span>
										{/if}
										{#if field(s, 'award_amount_usd')}
											<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">${field(s, 'award_amount_usd')}</span>
										{/if}
										{#if field(s, 'application_deadline')}
											<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">
												{t('scholarships.deadline', 'Deadline')}: {field(s, 'application_deadline')}
											</span>
										{/if}
									</div>
								</div>
								{#if authed}
									<button
										type="button"
										onclick={() => toggleSave(s)}
										class="shrink-0 rounded-[var(--radius)] border border-[var(--color-border)] p-2 text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-subtle)]"
										aria-label={savedIds.has(s.guide_id) ? t('common.saved', 'Saved') : t('common.save', 'Save')}
									>
										{#if savedIds.has(s.guide_id)}
											<BookmarkCheck size={16} class="text-[var(--color-primary)]" />
										{:else}
											<Bookmark size={16} />
										{/if}
									</button>
								{/if}
							</div>
						</div>
					{/each}
				</div>

				{#if data.total > data.pageSize}
					<div class="mt-4">
						<PaginationControls
							page={data.currentPage}
							totalPages={Math.ceil(data.total / data.pageSize)}
							onPageChange={(p) => {
								const params = new URLSearchParams(page.url.searchParams);
								params.set('page', String(p));
								goto(`/scholarships?${params.toString()}`);
							}}
						/>
					</div>
				{/if}
			{/if}
		</div>
	{/if}
</div>

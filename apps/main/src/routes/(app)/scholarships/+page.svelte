<script lang="ts">
	import { t, i18n } from '#lib/i18n/index.js';
	import EntityHoverChip from '#lib/components/ui/EntityHoverChip.svelte';
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { enhance } from '$app/forms';
	import { GraduationCap, Lock, Bookmark, BookmarkCheck, ClipboardList, GitCompare, Loader2, ExternalLink, Clock, ShieldCheck, ShieldAlert, Search, Globe, ChevronRight } from 'lucide-svelte';
	import { Filter } from 'lucide-svelte';
	import { EmptyState, EntityHero, ListPageBar, PaginationControls, SearchInput, AutocompleteSelect, SelectField, UrlTabs, type SearchSuggestion } from '#lib/components/ui/index.js';
	import { GuideSlideOver, Modal, type GuideContent } from '#lib/components/ui/index.js';
	import HeroArtScholarships from '#lib/components/heroes/HeroArtScholarships.svelte';
	import { formatMetric } from '#lib/components/hub/charts/format.js';
		import Autocomplete from '#lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '#lib/components/create/Autocomplete.svelte';
	import { api } from '#lib/api/client.js';
	import { goto } from '$app/navigation';
	import type { Scholarship } from '#lib/types/index.js';
	import type { PageData, ActionData } from './$types';
import { safeJsonLdScript } from '#lib/utils/safeHtml.js';

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
		notes?: string | null;
		remind_before_days?: number | null;
		updated_at?: string;
	}

	let { data, form }: { data: PageData; form?: ActionData } = $props();

	const MAX_COMPARE = 4;

	const title = $derived(t('tools.scholarships', 'Scholarship Finder — {site}'));
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
	let minAmount = $state(page.url.searchParams.get('min_amount') ?? '');
	let maxAmount = $state(page.url.searchParams.get('max_amount') ?? '');
	let renewable = $state(page.url.searchParams.get('renewable') ?? '');
	let deadlineFrom = $state(page.url.searchParams.get('deadline_from') ?? '');
	let deadlineTo = $state(page.url.searchParams.get('deadline_to') ?? '');
	let includeExpired = $state(page.url.searchParams.get('include_expired') === 'true');
	let sort = $state(page.url.searchParams.get('sort') ?? '');
	let savedIds = $state<Set<string>>(untrack(() => new Set(data.savedIds ?? [])));

	// Resync local state when the server load re-runs (filter/pagination nav,
	// back/forward). The URL is the source of truth for filters; saved tracker
	// state comes from data. Local optimistic mutations (toggleSave) don't
	// change data.savedIds/data.savedScholarships, so they are not clobbered.
	$effect(() => {
		const sp = page.url.searchParams;
		q = sp.get('q') ?? '';
		country = sp.get('country') ?? '';
		level = sp.get('level') ?? '';
		minAmount = sp.get('min_amount') ?? '';
		maxAmount = sp.get('max_amount') ?? '';
		renewable = sp.get('renewable') ?? '';
		deadlineFrom = sp.get('deadline_from') ?? '';
		deadlineTo = sp.get('deadline_to') ?? '';
		includeExpired = sp.get('include_expired') === 'true';
		sort = sp.get('sort') ?? '';
	});
	$effect(() => {
		savedScholarships = data.savedScholarships ?? [];
		savedIds = new Set(data.savedIds ?? []);
	});

	// JCE quick-comparison selection (Section 73): select up to 4 scholarships.
	let selectedIds = $state<Set<string>>(new Set());
	let savingJCE = $state(false);

	const selectedScholarships = $derived(scholarships.filter((s) => selectedIds.has(s.guide_id)));
	const canCompare = $derived(selectedIds.size >= 2 && selectedIds.size <= MAX_COMPARE);

	function toggleCompare(s: Scholarship) {
		const next = new Set(selectedIds);
		if (next.has(s.guide_id)) {
			next.delete(s.guide_id);
		} else if (next.size < MAX_COMPARE) {
			next.add(s.guide_id);
		}
		selectedIds = next;
	}

	// Rule 16: application status options come from the backend API and are
	// passed in via PageData. Labels are derived from i18n keys by status value.
	const statusOptions = $derived(
		data.statuses.map((value) => ({
			value,
			label: t(`scholarships.status_${value}`, value.charAt(0).toUpperCase() + value.slice(1))
		}))
	);

	const statusBadgeColor = (status: string): string => {
		if (status === 'accepted') return 'bg-[var(--color-success)] text-white';
		if (status === 'rejected' || status === 'withdrawn') return 'bg-[var(--color-danger)] text-white';
		if (status === 'applied' || status === 'researching') return 'bg-[var(--color-info)] text-white';
		return 'bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]';
	};

	function formatDate(date: string | null | undefined): string {
		if (!date) return '—';
		const d = new Date(date);
		if (Number.isNaN(d.getTime())) return date;
		return d.toLocaleDateString(i18n.locale, { year: 'numeric', month: 'short', day: 'numeric' });
	}

	let amountError = $state('');

	function applyFilters() {
		amountError = '';
		if (minAmount !== '' && Number.isNaN(Number(minAmount))) {
			amountError = t('scholarships.invalid_amount', 'Min and max amounts must be numbers.');
			return;
		}
		if (maxAmount !== '' && Number.isNaN(Number(maxAmount))) {
			amountError = t('scholarships.invalid_amount', 'Min and max amounts must be numbers.');
			return;
		}
		if (minAmount !== '' && maxAmount !== '' && Number(minAmount) > Number(maxAmount)) {
			amountError = t('scholarships.invalid_amount_range', 'Min amount cannot exceed max amount.');
			return;
		}
		const params = new URLSearchParams();
		if (q) params.set('q', q);
		if (country) params.set('country', country);
		if (level) params.set('level', level);
		if (minAmount) params.set('min_amount', minAmount);
		if (maxAmount) params.set('max_amount', maxAmount);
		if (renewable) params.set('renewable', renewable);
		if (deadlineFrom) params.set('deadline_from', deadlineFrom);
		if (deadlineTo) params.set('deadline_to', deadlineTo);
		if (sort) params.set('sort', sort);
		if (includeExpired) params.set('include_expired', 'true');
		goto(`/scholarships?${params.toString()}`, { reset: false });
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
		level = '';
		minAmount = '';
		maxAmount = '';
		renewable = '';
		deadlineFrom = '';
		deadlineTo = '';
		sort = '';
		includeExpired = false;
		goto('/scholarships');
	}

		// Level options = distinct "Level of Study" values — API list first
	// (/scholarships/levels), fallback to the loaded page's values (Rule 16).
	const levelSelectOptions = $derived.by(() => {
		const src = (data.levels?.length ?? 0) > 0
			? data.levels!
			: [...new Set(scholarships.map((s) => s.level_of_study).filter(Boolean))];
		// Case-insensitive dedupe — the API returns "Masters"/"masters" variants.
		const seen = new Map<string, string>();
		for (const l of src) if (!seen.has(l.toLowerCase())) seen.set(l.toLowerCase(), l);
		return [
			{ value: '', label: t('scholarships.level_all', 'All levels') },
			...[...seen.values()].map((l) => ({ value: l, label: l }))
		];
	});

const hasActiveFilters = $derived(
		!!q || !!country || !!level || !!minAmount || !!maxAmount || !!renewable || !!deadlineFrom || !!deadlineTo || !!sort || includeExpired
	);

	let filtersModalOpen = $state(false);
	let searchModalOpen = $state(false);
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
					status: data.statuses[0] ?? 'saved',
					notes: '',
					remind_before_days: data.defaultReminderDays,
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
		const res = await api.getEnvelope<Scholarship[]>(`/scholarships?q=${encodeURIComponent(q)}&limit=8`);
		if (res.error || !res.data) return [];
		void res.envelope?.meta;
		// The backend returns the scholarship array directly in `data`
		// (`{ data: [...], meta: {...} }`), not wrapped in a `scholarships`
		// key — reading `res.data.scholarships` here made suggestions empty.
		return (res.data ?? []).map((s) => ({
			id: s.guide_id,
			title: s.title,
			url: `/scholarships/${s.guide_id}`,
			excerpt: s.summary
		}));
	}

	// Country filter — keyed by the canonical country name used by the
	// scholarship ILIKE filter. Filters the full /geo/countries list client-side.
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
							url: `${page.url.origin}/scholarships/${s.guide_id}`
						}
					}))
				})
			: ''
	);

	// "How it works" slide-over — markdown panel + CTA repeater persisted to
	// public system settings (scholarships.guide) via PUT /admin/settings/general.
	let guideOpen = $state(false);
	let guideOverride = $state<GuideContent | null>(null);
	const siteSettings = $derived((page.data.siteSettings ?? {}) as Record<string, unknown>);
	const storedGuide = $derived.by((): Partial<GuideContent> => {
		const raw = siteSettings['scholarships.guide'];
		if (raw && typeof raw === 'object' && !Array.isArray(raw)) return raw as Partial<GuideContent>;
		return {};
	});
	const effectiveGuide = $derived<GuideContent>({
		title: guideOverride?.title ?? storedGuide.title ?? t('common.how_it_works', 'How it works'),
		body: guideOverride?.body ?? storedGuide.body ?? t('scholarships.guide_body', ''),
		ctas: guideOverride?.ctas ?? storedGuide.ctas ?? [{ title: t('guide_panel.read_full_guide', 'Read the full guide'), url: '/guides' }]
	});

	async function saveGuide(v: GuideContent): Promise<string | null> {
		const res = await api.put('/admin/settings/general', { scholarships_guide: v });
		if (res.error) return res.error.message || t('common.error', 'Something went wrong');
		guideOverride = v;
		return null;
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	{#if jsonLd}
		{@html safeJsonLdScript(jsonLd)}
	{/if}
</svelte:head>

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 48rem)/2))">
	<!-- Header — EntityHero directory panel -->
	<EntityHero
		class="mt-4"
		icon={GraduationCap}
		accent="var(--color-primary)"
		heading={t('tools.scholarships', 'Scholarship Finder')}
		description={t('scholarships.hero_desc', 'Find and track scholarships, fellowships, and grants for your study-abroad journey.')}
		crumbs={[
			{ label: t('common.home', 'Home'), href: '/' },
			{ label: t('tools.scholarships', 'Scholarships') }
		]}
		stats={[
			{ icon: GraduationCap, value: formatMetric(data.total), label: t('scholarships.stats_open', 'Scholarships') },
			{ icon: Globe, value: formatMetric(data.countries.length), label: t('scholarships.stats_countries', 'Countries') }
		]}
	>
		{#snippet art()}
			<HeroArtScholarships accent="var(--color-primary)" />
		{/snippet}
		{#snippet actions(c)}
			<div class="grid w-full grid-cols-2 gap-3 sm:flex sm:items-center">
			<a href="#scholarship-results" class="{c.primary} flex-1 justify-center whitespace-nowrap">
				<Search size={15} /> {t('scholarships.find', 'Find scholarships')}
			</a>
			{#if authed}
				<a href="/scholarships/saved" class="{c.secondary} flex-1 justify-center whitespace-nowrap">
					<Bookmark size={15} /> {t('scholarships.saved_cta', 'Saved scholarships')}
				</a>
			{/if}
			<button type="button" onclick={() => (guideOpen = true)} class="{c.ghost} flex-1 justify-center whitespace-nowrap">
				{t('common.how_it_works', 'How it works')}
				<ChevronRight size={14} class="rtl:-scale-x-100" />
				</button>
			</div>
		{/snippet}
	</EntityHero>

	{#if disabled}
		<div class="p-4" style="margin-inline: var(--page-gutter)">
			<EmptyState
				icon={Lock}
				title={t('tools.disabled_title', 'This tool is currently unavailable')}
				body={t('tools.disabled_body', 'Check back soon or contact support.')}
			/>
		</div>
	{:else}
<!-- Filters + Search on the right — the /sources pattern. -->
		<ListPageBar>
			<UrlTabs
				tabs={levelSelectOptions}
				value={level}
				onValueChange={(v) => { level = v; applyFilters(); }}
				urlParam="level"
				class="!border-b-0 !bg-transparent"
			/>
			{#snippet actions()}
				<button type="button" onclick={() => (filtersModalOpen = true)} class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]">
					{t('common.filters', 'Filters')}
					{#if hasActiveFilters}<span class="ms-0.5 h-2 w-2 rounded-full bg-[var(--color-primary)]" aria-hidden="true"></span>{/if}
					<Filter size={16} />
				</button>
				<button type="button" onclick={() => (searchModalOpen = true)} class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]">
					{t('common.search', 'Search')}
					{#if q.trim()}<span class="ms-0.5 h-2 w-2 rounded-full bg-[var(--color-primary)]" aria-hidden="true"></span>{/if}
					<Search size={16} />
				</button>
			{/snippet}
		</ListPageBar>

						<!-- Advanced filter panel -->
		<Modal bind:open={filtersModalOpen} title={t('common.filters', 'Filters')} icon={Filter} size="lg">
		{#snippet children()}
		<div>

			{#if amountError}
				<p class="mb-2 text-xs text-[var(--color-danger)]" role="alert">{amountError}</p>
				{/if}
			<form onsubmit={(e) => { e.preventDefault(); applyFilters(); }} class="grid grid-cols-1 gap-2 sm:grid-cols-2">
				<Autocomplete
					placeholder={t('scholarships.country', 'Country')}
					ariaLabel={t('scholarships.country', 'Country')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={fetchCountries}
					initialFetcher={fetchCountriesInitial}
					selectedId={country || null}
					selectedLabel={selectedCountryLabel}
					onSelect={(item) => { country = item.id; applyFilters(); }}
					onClear={() => { country = ''; applyFilters(); }}
				/>
				<input
					type="number"
					min="0"
					step="1"
					placeholder={t('scholarships.min_amount', 'Min amount (USD)')}
					aria-label={t('scholarships.min_amount', 'Min amount (USD)')}
					bind:value={minAmount}
					onchange={applyFilters}
					class="min-h-[44px] w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-[11px] text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
				/>
				<input
					type="number"
					min="0"
					step="1"
					placeholder={t('scholarships.max_amount', 'Max amount (USD)')}
					aria-label={t('scholarships.max_amount', 'Max amount (USD)')}
					bind:value={maxAmount}
					onchange={applyFilters}
					class="min-h-[44px] w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-[11px] text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
				/>
				<AutocompleteSelect
					ariaLabel={t('scholarships.renewable', 'Renewable')}
					value={renewable}
					options={[
						{ value: '', label: t('scholarships.renewable_any', 'Any renewable') },
						{ value: 'true', label: t('scholarships.renewable_yes', 'Yes') },
						{ value: 'false', label: t('scholarships.renewable_no', 'No') }
					]}
					onValueChange={(v) => {
						renewable = v;
						applyFilters();
					}}
				/>
				<fieldset class="sm:col-span-2">
					<legend class="mb-1 text-xs font-medium text-[var(--color-muted)]">{t('scholarships.deadline', 'Application deadline')}</legend>
					<div class="grid grid-cols-2 gap-2">
						<label class="flex flex-col gap-1">
							<span class="text-xs text-[var(--color-muted)]">{t('scholarships.deadline_from', 'From')}</span>
							<input
								type="date"
								bind:value={deadlineFrom}
								onchange={applyFilters}
								class="min-h-[44px] w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-[11px] text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
							/>
						</label>
						<label class="flex flex-col gap-1">
							<span class="text-xs text-[var(--color-muted)]">{t('scholarships.deadline_to', 'To')}</span>
							<input
								type="date"
								bind:value={deadlineTo}
								onchange={applyFilters}
								class="min-h-[44px] w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-[11px] text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
							/>
						</label>
					</div>
				</fieldset>
				<AutocompleteSelect
					ariaLabel={t('scholarships.sort', 'Sort by')}
					value={sort}
					options={[
						{ value: '', label: t('scholarships.sort_default', 'Default') },
						{ value: 'deadline_asc', label: t('scholarships.sort_deadline_asc', 'Deadline (earliest)') },
						{ value: 'amount_desc', label: t('scholarships.sort_amount_desc', 'Award amount (highest)') }
					]}
					onValueChange={(v) => {
						sort = v;
						applyFilters();
					}}
				/>
				<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
					<input
						type="checkbox"
						bind:checked={includeExpired}
						onchange={applyFilters}
						class="rounded border-[var(--color-border)] text-[var(--color-primary-text)] focus:ring-[var(--color-primary)]"
					/>
					{t('scholarships.include_expired', 'Include expired')}
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
		{/snippet}
	</Modal>

		<!-- Application tracker — Phase 33.04 -->
		{#if authed && savedScholarships.length > 0}
			<div class="border-b border-[var(--color-border)] px-4 py-3" style="margin-inline: var(--page-gutter)">
				<div class="mb-3 flex items-center justify-between">
					<h2 class="flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
						<ClipboardList size={16} />
						{t('scholarships.tracker', 'Application Tracker')}
					</h2>
					<a
						href="/scholarships/saved"
						class="text-xs text-[var(--color-primary-text)] hover:underline"
					>
						{t('scholarships.view_saved', 'View saved')}
					</a>
				</div>
				<div class="space-y-2">
					{#each savedScholarships as saved (saved.guide_id)}
						<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
							<div class="flex items-start justify-between gap-3">
								<div class="min-w-0 flex-1">
									<a
										href="/scholarships/{saved.guide_id}"
										class="truncate text-sm font-medium text-[var(--color-text)] hover:text-[var(--color-primary-text)]"
									>
										{saved.title}
									</a>
									<div class="mt-1 flex items-center gap-2">
										<span class="rounded-full px-2 py-0.5 text-xs font-medium {statusBadgeColor(saved.status)}">
											{statusOptions.find((o) => o.value === saved.status)?.label ?? saved.status}
										</span>
										{#if saved.updated_at}
											<span class="text-xs text-[var(--color-muted)]">{t('scholarships.updated', 'Updated')}: {formatDate(saved.updated_at)}</span>
										{/if}
									</div>
								</div>
								<AutocompleteSelect
									class="w-40 shrink-0"
									value={saved.status}
									options={statusOptions}
									onValueChange={(v) => updateStatus(saved, v)}
									ariaLabel={t('scholarships.status_label', 'Application status')}
									inputClass="text-xs"
								/>
							</div>
						</div>
					{/each}
				</div>
			</div>
		{/if}

		<!-- Results -->
		<div id="scholarship-results" class="p-4" style="margin-inline: var(--page-gutter)">
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
				<!-- Result rows — RadarItemRow styling: icon + title, clamped
				     summary, meta pills, and a badges row with the source trust tier. -->
				<div class="space-y-2">
					{#each scholarships as s (s.guide_id)}
						{@const selected = selectedIds.has(s.guide_id)}
						{@const trustTier = s.source?.trust_level ?? 0}
						{@const deadline = field(s, 'application_deadline')}
						<div class="group rounded-[var(--radius-card)] border p-4 transition-colors {selected ? 'border-[var(--color-primary)] bg-[var(--color-primary)]/5' : 'border-[var(--color-border)] bg-[var(--color-surface)] hover:border-[var(--color-primary)]/40 hover:bg-[var(--color-surface-subtle)]'}">
							<div class="flex items-start justify-between gap-3">
								<div class="min-w-0 flex-1">
									<div class="flex items-center gap-2">
										<GraduationCap size={16} class="shrink-0 text-[var(--color-primary-text)]" />
										<a
											href="/scholarships/{s.guide_id}"
											class="truncate text-sm font-medium text-[var(--color-text)] group-hover:text-[var(--color-primary-text)]"
										>
											{s.title}
										</a>
									</div>
									{#if s.summary}
										<p class="mt-1 line-clamp-2 text-xs text-[var(--color-muted)]">{s.summary}</p>
									{/if}
									<div class="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-[var(--color-muted)]">
										{#if field(s, 'countries_eligible') || s.country}
											<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">{field(s, 'countries_eligible') || s.country}</span>
										{/if}
										{#if s.institution}
											<span class="truncate">{s.institution}</span>
										{/if}
										{#if field(s, 'level_of_study') || s.level_of_study}
											<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">{field(s, 'level_of_study') || s.level_of_study}</span>
										{/if}
										{#if field(s, 'award_amount_usd')}
											<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">{field(s, 'award_amount_usd')}</span>
										{/if}
										{#if deadline || s.application_deadline}
											<span class="flex items-center gap-1">
												<Clock size={12} />
												{t('scholarships.deadline', 'Deadline')}: {deadline || s.application_deadline}
											</span>
										{/if}
									</div>
								</div>
								<div class="flex shrink-0 flex-col items-end gap-2">
									<label class="inline-flex items-center gap-1.5 text-xs text-[var(--color-text-subtle)]">
										<input
											type="checkbox"
											checked={selected}
											onchange={() => toggleCompare(s)}
											disabled={!selected && selectedIds.size >= MAX_COMPARE}
											class="h-4 w-4 rounded border-[var(--color-border)] accent-[var(--color-primary)]"
										/>
										{t('scholarships.compare_select', 'Compare')}
									</label>
									{#if authed}
										<button
											type="button"
											onclick={() => toggleSave(s)}
											class="shrink-0 rounded-[var(--radius)] border border-[var(--color-border)] p-2 text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-subtle)]"
											aria-label={savedIds.has(s.guide_id) ? t('common.saved', 'Saved') : t('common.save', 'Save')}
										>
											{#if savedIds.has(s.guide_id)}
												<BookmarkCheck size={16} class="text-[var(--color-primary-text)]" />
											{:else}
												<Bookmark size={16} />
											{/if}
										</button>
									{/if}
								</div>
							</div>

							<!-- Badges row: source trust tier + source attribution -->
							{#if trustTier > 0 || s.source?.slug}
								<div class="mt-3 flex flex-wrap items-center gap-2">
									{#if trustTier > 0}
										<span class="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium {trustTier <= 2 ? 'border-[var(--color-success)]/40 bg-[var(--color-success)]/10 text-[var(--color-success)]' : 'border-[var(--color-warning)]/40 bg-[var(--color-warning)]/10 text-[var(--color-warning)]'}">
											{#if trustTier <= 2}
												<ShieldCheck size={12} />
											{:else}
												<ShieldAlert size={12} />
											{/if}
											{trustTier <= 1
												? t('radar.trust_high', 'High trust')
												: trustTier <= 2
													? t('radar.trust_verified', 'Verified')
													: t('radar.trust_review', 'Needs review')}
										</span>
									{/if}
									{#if s.source?.slug}
										{@const source = s.source}
										<span class="inline-flex items-center gap-1 text-xs">
											<EntityHoverChip
												type="source"
												id={source.id}
												slug={source.slug}
												name={source.name}
												avatar={source.logo_url}
												class="inline-flex items-center gap-0.5 font-medium text-[var(--color-primary-text)] hover:underline"
											>
												{#snippet trigger()}
													{t('scholarships.source', 'Source')}: {source.name}
												{/snippet}
											</EntityHoverChip>
											{#if source.source_url}
												<a
													href={source.source_url}
													target="_blank"
													rel="noopener noreferrer"
													class="inline-flex items-center gap-0.5 text-[var(--color-muted)] hover:text-[var(--color-primary-text)]"
													aria-label={t('scholarships.original_source', 'View original source')}
												>
													<ExternalLink class="rtl:-scale-x-100" size={12} />
												</a>
											{/if}
										</span>
									{/if}
								</div>
							{/if}
						</div>
					{/each}
				</div>

				{#if form?.error}
					<div class="mb-4 rounded-[var(--radius)] border border-[var(--color-danger)]/30 bg-[var(--color-danger)]/10 p-3 text-sm text-[var(--color-danger)]">
						{form.message || t('scholarships.jce_error', 'Could not create comparison')}
					</div>
				{/if}

				<!-- Floating JCE compare bar -->
				{#if selectedIds.size > 0}
					<div class="sticky bottom-3 mt-4 flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2.5 shadow-sm">
						<span class="text-sm text-[var(--color-text-subtle)]">
							{selectedIds.size}/{MAX_COMPARE} {t('scholarships.selected', 'selected')}
						</span>
						<div class="flex items-center gap-2">
							<button
								type="button"
								onclick={() => (selectedIds = new Set())}
								class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)]"
							>
								{t('common.clear', 'Clear')}
							</button>

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
								{#each selectedScholarships as s (s.guide_id)}
									<input type="hidden" name="guide_id" value={s.guide_id} />
								{/each}
								<button
									type="submit"
									disabled={!canCompare || !authed}
									title={authed
										? t('scholarships.open_in_jce_hint', 'Open in JCE comparison')
										: t('scholarships.sign_in_to_compare', 'Sign in to compare')}
									class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50 disabled:cursor-not-allowed"
								>
									{#if savingJCE}
										<Loader2 size={14} class="animate-spin" />
									{:else}
										<GitCompare size={14} />
									{/if}
									{t('scholarships.compare_in_jce', 'Compare in JCE')}
								</button>
							</form>
						</div>
					</div>
				{/if}

				{#if data.total > data.pageSize}
					<div class="mt-4">
						<PaginationControls
							page={data.currentPage}
							totalPages={Math.ceil(data.total / data.pageSize)}
							onPageChange={(p) => {
								const params = new URLSearchParams(page.url.search);
								params.set('page', String(p));
								goto(`/scholarships?${params.toString()}`);
							}}
						/>
					</div>
				{/if}
			{/if}
		</div>

	<Modal bind:open={searchModalOpen} title={t('common.search', 'Search')} icon={Search} size="sm">
		{#snippet children()}
			<SearchInput
				bind:value={q}
				placeholder={t('scholarships.search', 'Search')}
				ariaLabel={t('tools.scholarships', 'Scholarship Finder')}
				fetcher={scholarshipSearchFetcher}
				onInput={queueApply}
				inputClass="min-h-[40px] py-2 focus:ring-2 focus:ring-[var(--color-primary)]"
			/>
		{/snippet}
	</Modal>
	{/if}
</div>

<!-- "How it works" slide-over — GuideSlideOver; admins edit inline (scholarships.guide). -->
<GuideSlideOver
	bind:open={guideOpen}
	title={effectiveGuide.title}
	body={effectiveGuide.body}
	ctas={effectiveGuide.ctas}
	canManage={data.canManage}
	onSave={saveGuide}
/>

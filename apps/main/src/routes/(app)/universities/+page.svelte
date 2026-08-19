<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { navigating } from '$app/state';
	import { GraduationCap, Users, FileText, Heart, LayoutGrid, List, X } from 'lucide-svelte';
	import {
		EmptyState,
		LoadingSkeleton,
		PaginationControls,
		SearchInput,
		SelectField,
		type SearchSuggestion
	} from '$lib/components/ui';
	import type { University } from '$lib/types';
	import type { PageData } from './$types';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
import { safeJsonLdScript } from '$lib/utils/safeHtml';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('universities.title', 'Universities — JapaConnect'));
	const description = $derived(
		t('universities.description', 'Explore universities relevant to the Nigerian diaspora community.')
	);

	// Server-paginated slice for the current page (filter/sort/page are
	// server-side — the backend honors ?q=, ?country=, ?sort=, ?page=, ?limit=).
	const paged = $derived<University[]>(data.universities);
	const total = $derived(data.meta?.total ?? data.universities.length);
	const totalPages = $derived(data.meta?.total_pages ?? 1);
	const currentPage = $derived(data.meta?.page ?? (parseInt(page.url.searchParams.get('page') ?? '1', 10) || 1));

	// URL is the source of truth for filter / sort / page state. The country
	// filter falls back to the geo-aware default (ADR-064) resolved server-side
	// when no explicit ?country= param is present — "a UK visitor sees UK
	// universities first". An explicit param always wins.
	const qParam = $derived(page.url.searchParams.get('q') ?? '');
	const countryParam = $derived(page.url.searchParams.get('country') ?? '');
	const effectiveCountry = $derived(countryParam || data.countryDefault || '');
	const sortParam = $derived(page.url.searchParams.get('sort') ?? 'newest');

	// Country options are derived from the distinct geo_code values present in
	// the current page slice. When the page has no geo_codes the dropdown
	// degrades to a single "All countries" entry. (For a full country list the
	// /geo/countries endpoint is the canonical source; here we keep the dropdown
	// lightweight and derived from what's on screen.)
	const countryOptions = $derived.by(() => {
		const codes = [...new Set(paged.map((u) => u.geo_code).filter((c): c is string => !!c))];
		return [
			{ value: '', label: t('universities.all_countries', 'All countries') },
			...codes.map((c) => ({ value: c, label: c }))
		];
	});

	const sortOptions = $derived([
		{ value: 'newest', label: t('universities.sort_newest', 'Newest') },
		{ value: 'name', label: t('universities.sort_name', 'Name (A–Z)') },
		{ value: 'members', label: t('universities.sort_members', 'Most members') },
		{ value: 'followers', label: t('universities.sort_followers', 'Most followers') },
		{ value: 'posts', label: t('universities.sort_posts', 'Most posts') }
	]);

	const hasActiveFilters = $derived(!!qParam || !!effectiveCountry);
	const showingStart = $derived(total === 0 ? 0 : (currentPage - 1) * data.pageSize + 1);
	const showingEnd = $derived(Math.min(currentPage * data.pageSize, total));

	// Form input state — kept in sync with URL params so back/forward navigation
	// restores the visible field values.
	let qInput = $state(page.url.searchParams.get('q') ?? '');
	let countryInput = $state(page.url.searchParams.get('country') ?? untrack(() => data.countryDefault) ?? '');
	let viewMode = $state<'grid' | 'list'>('grid');

	// Sync local form state whenever the URL params change (back/forward, clear).
	$effect(() => {
		qInput = page.url.searchParams.get('q') ?? '';
		countryInput = page.url.searchParams.get('country') ?? data.countryDefault ?? '';
	});

	// Build a URLSearchParams from the current filter state, resetting to page 1.
	function buildParams(overrides: Record<string, string> = {}): URLSearchParams {
		const params = new URLSearchParams();
		const q = overrides.q ?? qInput;
		const country = overrides.country ?? countryInput;
		const sort = overrides.sort ?? sortParam;
		if (q) params.set('q', q);
		if (country) params.set('country', country);
		if (sort && sort !== 'newest') params.set('sort', sort);
		return params;
	}

	function applySearch() {
		const params = buildParams();
		goto(`/universities${params.toString() ? `?${params.toString()}` : ''}`, { keepFocus: true });
	}

	function setSort(value: string) {
		const params = buildParams({ sort: value });
		goto(`/universities${params.toString() ? `?${params.toString()}` : ''}`, { keepFocus: true });
	}

	function setCountry(value: string) {
		countryInput = value;
		const params = buildParams({ country: value });
		goto(`/universities${params.toString() ? `?${params.toString()}` : ''}`, { keepFocus: true });
	}

	function clearFilters() {
		qInput = '';
		countryInput = '';
		const params = new URLSearchParams();
		if (sortParam && sortParam !== 'newest') params.set('sort', sortParam);
		goto(`/universities${params.toString() ? `?${params.toString()}` : ''}`);
	}

	function goToPage(p: number) {
		const params = buildParams();
		if (p > 1) params.set('page', String(p));
		goto(`/universities${params.toString() ? `?${params.toString()}` : ''}`);
	}

	// Autocomplete fetcher — filters the already-loaded page slice (no API call).
	// The full search is performed server-side on submit; this only suggests
	// matches from the current page for quick navigation.
	async function uniSearchFetcher(query: string): Promise<SearchSuggestion[]> {
		const q = query.trim().toLowerCase();
		if (!q) return [];
		return paged
			.filter(
				(u) =>
					u.name.toLowerCase().includes(q) ||
					(u.initials?.toLowerCase().includes(q) ?? false)
			)
			.slice(0, 8)
			.map((u) => ({
				id: u.id,
				title: u.initials ? `${u.initials} — ${u.name}` : u.name,
				url: `/universities/${u.slug}`,
				excerpt: u.geo_code ?? undefined,
				type: t('universities.suggestion_type', 'University')
			}));
	}

	// JSON-LD: ItemList of EducationalOrganization for the current page view.
	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'ItemList',
			itemListElement: paged.map((uni, i) => ({
				'@type': 'ListItem',
				position: showingStart + i,
				item: {
					'@type': 'EducationalOrganization',
					name: uni.name,
					url: `${page.url.origin}/universities/${uni.slug}`
				}
			}))
		})
	);

	// Autocomplete fetchers — filter the already-derived country options client-side.
	// The "All countries" option (id: '') is always included at the top.
	function fetchCountries(q: string): Promise<AutocompleteItem[]> {
		const allLabel = t('universities.all_countries', 'All countries');
		const query = q.toLowerCase();
		const allItem: AutocompleteItem = { id: '', label: allLabel };
		if (query === '') {
			return Promise.resolve([allItem, ...countryOptions.filter((o) => o.value !== '').map((o) => ({ id: o.value, label: o.label }))]);
		}
		const matchesAll = allLabel.toLowerCase().includes(query);
		const filtered = countryOptions
			.filter((o) => o.value !== '' && (o.value.toLowerCase().includes(query) || o.label.toLowerCase().includes(query)))
			.map((o) => ({ id: o.value, label: o.label }));
		return Promise.resolve(matchesAll ? [allItem, ...filtered] : filtered);
	}
	function fetchCountriesInitial(): Promise<AutocompleteItem[]> {
		return fetchCountries('');
	}
	const selectedCountryLabel = $derived(
		countryInput
			? countryOptions.find((o) => o.value === countryInput)?.label ?? countryInput
			: t('universities.all_countries', 'All countries')
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	{@html safeJsonLdScript(jsonLd)}
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<div class="flex items-end justify-between gap-3 border-b border-[var(--color-border)] px-4 py-3">
		<div class="min-w-0">
			<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
				<GraduationCap size={20} /> {t('universities.heading', 'Universities')}
			</h1>
			<p class="mt-1 text-sm text-[var(--color-muted)]">{description}</p>
		</div>
		<!-- Grid / list view toggle -->
		<div class="flex shrink-0 items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] p-0.5">
			<button
				type="button"
				onclick={() => (viewMode = 'grid')}
				class="inline-flex h-7 w-7 items-center justify-center rounded-[calc(var(--radius)-2px)] transition-colors {viewMode ===
				'grid'
					? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)]'
					: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
				aria-label={t('universities.view_grid', 'Grid view')}
				aria-pressed={viewMode === 'grid'}
			>
				<LayoutGrid size={15} />
			</button>
			<button
				type="button"
				onclick={() => (viewMode = 'list')}
				class="inline-flex h-7 w-7 items-center justify-center rounded-[calc(var(--radius)-2px)] transition-colors {viewMode ===
				'list'
					? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)]'
					: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
				aria-label={t('universities.view_list', 'List view')}
				aria-pressed={viewMode === 'list'}
			>
				<List size={15} />
			</button>
		</div>
	</div>

	<!-- Filter / search / sort bar -->
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<form
			onsubmit={(e) => {
				e.preventDefault();
				applySearch();
			}}
			class="flex flex-col gap-2 lg:flex-row lg:items-center"
		>
			<div class="min-w-0 flex-1">
				<SearchInput
					bind:value={qInput}
					placeholder={t('universities.search', 'Search universities…')}
					ariaLabel={t('universities.heading', 'Universities')}
					fetcher={uniSearchFetcher}
					inputClass="focus:ring-2 focus:ring-[var(--color-primary)]"
				/>
			</div>
			<div class="flex gap-2">
				<div class="w-40">
					<Autocomplete
						placeholder={t('universities.all_countries', 'All countries')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchCountries}
						initialFetcher={fetchCountriesInitial}
						selectedId={countryInput || null}
						selectedLabel={selectedCountryLabel}
						onSelect={(item) => setCountry(item.id)}
						onClear={() => setCountry('')}
					/>
				</div>
				<div class="w-44">
					<SelectField
						value={sortParam}
						options={sortOptions}
						placeholder={t('universities.sort_newest', 'Newest')}
						onValueChange={setSort}
					/>
				</div>
				<button
					type="submit"
					class="shrink-0 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:opacity-90"
				>
					{t('common.filter', 'Filter')}
				</button>
			</div>
		</form>

		{#if hasActiveFilters}
			<div class="mt-2">
				<button
					type="button"
					onclick={clearFilters}
					class="inline-flex items-center gap-1 text-xs text-[var(--color-muted)] underline hover:text-[var(--color-text)]"
				>
					<X size={12} /> {t('common.clear_filters', 'Clear filters')}
				</button>
			</div>
		{/if}
	</div>

	<!-- Results -->
	<div class="p-3">
		{#if navigating.to}
			<LoadingSkeleton variant="card" />
		{:else if total === 0}
			<EmptyState
				icon={GraduationCap}
				title={t('universities.empty', 'No universities found')}
				body={t('universities.empty_body', 'Try adjusting your search or filters.')}
			/>
		{:else}
			<p class="mb-3 text-xs text-[var(--color-muted)]">
				{t('common.showing', 'Showing {start}-{end} of {total}')
					.replace('{start}', String(showingStart))
					.replace('{end}', String(showingEnd))
					.replace('{total}', String(total))}
			</p>

			{#if viewMode === 'grid'}
				<div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
					{#each paged as uni (uni.id)}
						<a
							href="/universities/{uni.slug}"
							class="flex items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							<span
								class="grid h-12 w-12 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]"
							>
								<GraduationCap size={22} />
							</span>
							<span class="min-w-0 flex-1">
								<span class="flex items-center gap-2">
									{#if uni.initials}
										<span class="shrink-0 rounded bg-[var(--color-primary)]/10 px-1.5 py-0.5 text-xs font-semibold text-[var(--color-primary)]"
											>{uni.initials}</span>
									{/if}
									<span class="block truncate text-sm font-medium text-[var(--color-text)]"
										>{uni.name}</span>
								</span>
								<span
									class="mt-1 flex flex-wrap items-center gap-3 text-xs text-[var(--color-muted)]"
								>
									<span class="flex items-center gap-1"><Users size={12} /> {uni.member_count}</span>
									<span class="flex items-center gap-1"
										><FileText size={12} /> {uni.post_count}</span>
									<span class="flex items-center gap-1"
										><Heart size={12} /> {uni.follower_count}</span>
								</span>
							</span>
						</a>
					{/each}
				</div>
			{:else}
				<div class="overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)]">
					<table class="w-full text-left text-sm">
						<thead class="bg-[var(--color-surface-subtle)] text-xs uppercase text-[var(--color-muted)]">
							<tr>
								<th class="px-4 py-2 font-medium">{t('universities.col_name', 'Name')}</th>
								<th class="px-4 py-2 text-center font-medium">{t('universities.col_initials', 'Initials')}</th>
								<th class="px-4 py-2 text-right font-medium">{t('universities.col_members', 'Members')}</th>
								<th class="px-4 py-2 text-right font-medium">{t('universities.col_posts', 'Posts')}</th>
								<th class="px-4 py-2 text-right font-medium">{t('universities.col_followers', 'Followers')}</th>
							</tr>
						</thead>
						<tbody>
							{#each paged as uni (uni.id)}
								<tr class="border-t border-[var(--color-border)] transition-colors hover:bg-[var(--color-surface-subtle)]">
									<td class="px-4 py-2.5">
										<a
											href="/universities/{uni.slug}"
											class="flex items-center gap-2 text-[var(--color-text)] hover:underline"
										>
											<GraduationCap size={16} class="shrink-0 text-[var(--color-text-subtle)]" />
											<span class="truncate">{uni.name}</span>
										</a>
									</td>
									<td class="px-4 py-2.5 text-center">
										{#if uni.initials}
											<span class="rounded bg-[var(--color-primary)]/10 px-1.5 py-0.5 text-xs font-semibold text-[var(--color-primary)]"
												>{uni.initials}</span>
										{/if}
									</td>
									<td class="px-4 py-2.5 text-right tabular-nums text-[var(--color-muted)]">{uni.member_count}</td>
									<td class="px-4 py-2.5 text-right tabular-nums text-[var(--color-muted)]">{uni.post_count}</td>
									<td class="px-4 py-2.5 text-right tabular-nums text-[var(--color-muted)]">{uni.follower_count}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}

			{#if totalPages > 1}
				<div class="mt-4 flex justify-center">
					<PaginationControls
						page={currentPage}
						totalPages={totalPages}
						onPageChange={goToPage}
					/>
				</div>
			{/if}
		{/if}
	</div>
</div>

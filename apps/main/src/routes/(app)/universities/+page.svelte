<script lang="ts">
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

	let { data }: { data: PageData } = $props();

	const title = $derived(t('universities.title', 'Universities — JapaConnect'));
	const description = $derived(
		t('universities.description', 'Explore universities relevant to the Nigerian diaspora community.')
	);

	// Full catalog — filtered / sorted / paginated client-side (see +page.server.ts).
	const all = $derived<University[]>(data.universities);

	// URL is the source of truth for filter / sort / page state. The country
	// filter falls back to the geo-aware default (ADR-064) resolved server-side
	// when no explicit ?country= param is present — "a UK visitor sees UK
	// universities first". An explicit param always wins.
	const qParam = $derived(page.url.searchParams.get('q') ?? '');
	const countryParam = $derived(page.url.searchParams.get('country') ?? '');
	const effectiveCountry = $derived(countryParam || data.countryDefault || '');
	const sortParam = $derived(page.url.searchParams.get('sort') ?? 'name');
	const pageParam = $derived(parseInt(page.url.searchParams.get('page') ?? '1', 10) || 1);

	// Country options derived from the distinct geo_code values present in the
	// catalog. When no universities carry a geo_code the dropdown degrades to a
	// single "All countries" entry.
	const countryOptions = $derived.by(() => {
		const codes = [...new Set(all.map((u) => u.geo_code).filter((c): c is string => !!c))];
		return [
			{ value: '', label: t('universities.all_countries', 'All countries') },
			...codes.map((c) => ({ value: c, label: c }))
		];
	});

	const sortOptions = $derived([
		{ value: 'name', label: t('universities.sort_name', 'Name (A–Z)') },
		{ value: 'members', label: t('universities.sort_members', 'Most members') },
		{ value: 'followers', label: t('universities.sort_followers', 'Most followers') },
		{ value: 'posts', label: t('universities.sort_posts', 'Most posts') },
		{ value: 'newest', label: t('universities.sort_newest', 'Newest') }
	]);

	// Filter → Sort → Paginate pipeline.
	const filtered = $derived.by(() => {
		let list = all;
		const q = qParam.trim().toLowerCase();
		if (q) list = list.filter((u) => u.name.toLowerCase().includes(q));
		if (effectiveCountry) list = list.filter((u) => u.geo_code === effectiveCountry);
		return list;
	});

	const sorted = $derived.by(() => {
		const list = [...filtered];
		switch (sortParam) {
			case 'members':
				list.sort((a, b) => b.member_count - a.member_count);
				break;
			case 'followers':
				list.sort((a, b) => b.follower_count - a.follower_count);
				break;
			case 'posts':
				list.sort((a, b) => b.post_count - a.post_count);
				break;
			case 'newest':
				list.sort((a, b) => b.created_at.localeCompare(a.created_at));
				break;
			default:
				list.sort((a, b) => a.name.localeCompare(b.name));
		}
		return list;
	});

	const totalPages = $derived(Math.max(1, Math.ceil(sorted.length / data.pageSize)));
	const currentPage = $derived(Math.min(pageParam, totalPages));
	const paged = $derived(
		sorted.slice((currentPage - 1) * data.pageSize, currentPage * data.pageSize)
	);

	const hasActiveFilters = $derived(!!qParam || !!effectiveCountry);
	const showingStart = $derived(sorted.length === 0 ? 0 : (currentPage - 1) * data.pageSize + 1);
	const showingEnd = $derived(Math.min(currentPage * data.pageSize, sorted.length));

	// Form input state — kept in sync with URL params so back/forward navigation
	// restores the visible field values.
	let qInput = $state(page.url.searchParams.get('q') ?? '');
	let countryInput = $state(page.url.searchParams.get('country') ?? data.countryDefault ?? '');
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
		if (sort && sort !== 'name') params.set('sort', sort);
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
		if (sortParam && sortParam !== 'name') params.set('sort', sortParam);
		goto(`/universities${params.toString() ? `?${params.toString()}` : ''}`);
	}

	function goToPage(p: number) {
		const params = buildParams();
		if (p > 1) params.set('page', String(p));
		goto(`/universities${params.toString() ? `?${params.toString()}` : ''}`);
	}

	// Autocomplete fetcher — filters the already-loaded catalog (no API call).
	async function uniSearchFetcher(query: string): Promise<SearchSuggestion[]> {
		const q = query.trim().toLowerCase();
		if (!q) return [];
		return all
			.filter((u) => u.name.toLowerCase().includes(q))
			.slice(0, 8)
			.map((u) => ({
				id: u.id,
				title: u.name,
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
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	{@html `<script type="application/ld+json">${jsonLd}</script>`}
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
					<SelectField
						value={countryInput}
						options={countryOptions}
						placeholder={t('universities.all_countries', 'All countries')}
						onValueChange={setCountry}
					/>
				</div>
				<div class="w-44">
					<SelectField
						value={sortParam}
						options={sortOptions}
						placeholder={t('universities.sort_name', 'Name (A–Z)')}
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
		{:else if sorted.length === 0}
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
					.replace('{total}', String(sorted.length))}
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
								<span class="block truncate text-sm font-medium text-[var(--color-text)]"
									>{uni.name}</span>
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

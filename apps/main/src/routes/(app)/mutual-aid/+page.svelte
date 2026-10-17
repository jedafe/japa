<script lang="ts">
	// Ref: japaconnect-engineering/sections/85-mutual-aid-board.md
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { i18n } from '$lib/i18n';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { HandHeart, Plus, MapPin, Clock, X } from 'lucide-svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { ListPageBar, UrlTabs, type UrlTab, FilterToggle, PageAction } from '$lib/components/ui';
	import { relativeTime } from '$lib/feed/time';
	import { mutualAidPostTypeLabel, mutualAidCategoryLabel } from '$lib/enumOptions';
	import { staticAutocompleteFetcher } from '$lib/autocomplete';
	import type { MutualAidPost, MutualAidPostType, MutualAidCategory } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const posts = $derived(data.posts);
	const meta = $derived(data.meta);
	const totalPages = $derived(meta?.total_pages ?? 1);
	const currentPage = $derived(data.filters.page);

	// Filter bar state — seeded from the server-provided filter values.
	let dest = $state(untrack(() => data.filters.dest));
	let category = $state(untrack(() => data.filters.category));

	// Re-seed when the server payload changes (e.g. browser back/forward).
	$effect(() => {
		dest = data.filters.dest;
		category = data.filters.category;
	});

	const categoryItems = $derived<AutocompleteItem[]>(
		(data.categories ?? []).map((c) => ({ id: c, label: mutualAidCategoryLabel(t, c) }))
	);
	const categoryFetcher = $derived(staticAutocompleteFetcher(categoryItems));
	const selectedCategoryLabel = $derived(
		category ? (categoryItems.find((c) => c.id === category)?.label ?? category) : null
	);

	const tabs = $derived<UrlTab[]>([
		{ value: 'all', label: t('common.all', 'All') },
		...(data.postTypes ?? []).map((v) => ({ value: v, label: mutualAidPostTypeLabel(t, v) }))
	]);
	const activeTab = $derived(data.filters.type || 'all');

	let showFilters = $state(false);
	$effect(() => {
		if (hasFilters) showFilters = true;
	});

	function selectType(v: string) {
		const params = new URLSearchParams();
		if (data.filters.dest) params.set('dest', data.filters.dest);
		if (data.filters.category) params.set('category', data.filters.category);
		if (data.filters.sort && data.filters.sort !== 'latest') params.set('sort', data.filters.sort);
		if (v !== 'all') params.set('type', v);
		const qs = params.toString();
		goto(`/mutual-aid${qs ? `?${qs}` : ''}`);
	}

	const title = $derived(t('mutual_aid.list_title', 'Mutual Aid — {site}'));
	const description = $derived(
		t(
			'mutual_aid.list_description',
			'Community help requests and offers for the Nigerian diaspora.'
		)
	);

	function applyFilters(e?: SubmitEvent) {
		e?.preventDefault();
		const params = new URLSearchParams();
		if (dest.trim()) params.set('dest', dest.trim());
		if (data.filters.type) params.set('type', data.filters.type);
		if (category) params.set('category', category);
		if (data.filters.sort) params.set('sort', data.filters.sort);
		const qs = params.toString();
		goto(`/mutual-aid${qs ? `?${qs}` : ''}`);
	}

	function clearFilters() {
		dest = '';
		category = '';
		goto('/mutual-aid');
	}

	const hasFilters = $derived(!!data.filters.dest || !!data.filters.category);

	function postTypeBadgeClass(pt: MutualAidPostType): string {
		const base = 'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-semibold';
		switch (pt) {
			case 'need':
				return `${base} bg-red-500/10 text-red-600`;
			case 'offer':
				return `${base} bg-green-500/10 text-green-600`;
			case 'skill_swap':
				return `${base} bg-blue-500/10 text-blue-600`;
			default:
				return `${base} bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]`;
		}
	}

	function postTypeLabel(pt: MutualAidPostType): string {
		switch (pt) {
			case 'need':
				return t('mutual_aid.type_need', 'Need');
			case 'offer':
				return t('mutual_aid.type_offer', 'Offer');
			case 'skill_swap':
				return t('mutual_aid.type_skill_swap', 'Skill swap');
			default:
				return pt;
		}
	}

	function categoryLabel(cat: MutualAidCategory): string {
		const map: Record<MutualAidCategory, string> = {
			housing: t('mutual_aid.cat_housing', 'Housing'),
			transport: t('mutual_aid.cat_transport', 'Transport'),
			documents: t('mutual_aid.cat_documents', 'Documents'),
			job_search: t('mutual_aid.cat_job_search', 'Job search'),
			language: t('mutual_aid.cat_language', 'Language'),
			social: t('mutual_aid.cat_social', 'Social'),
			other: t('mutual_aid.cat_other', 'Other')
		};
		return map[cat] ?? cat;
	}

	function statusBadgeClass(status: string): string {
		const base = 'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium';
		switch (status) {
			case 'active':
				return `${base} bg-green-500/10 text-green-600`;
			case 'fulfilled':
				return `${base} bg-[var(--color-primary)]/10 text-[var(--color-primary)]`;
			case 'expired':
				return `${base} bg-[var(--color-surface-subtle)] text-[var(--color-muted)]`;
			default:
				return `${base} bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]`;
		}
	}

	function statusLabel(status: string): string {
		switch (status) {
			case 'active':
				return t('mutual_aid.status_active', 'Active');
			case 'fulfilled':
				return t('mutual_aid.status_fulfilled', 'Fulfilled');
			case 'expired':
				return t('mutual_aid.status_expired', 'Expired');
			default:
				return status;
		}
	}

	function excerpt(text: string, max = 160): string {
		if (text.length <= max) return text;
		return text.slice(0, max).trimEnd() + '…';
	}

	function pageHref(p: number): string {
		const params = new URLSearchParams();
		if (data.filters.dest) params.set('dest', data.filters.dest);
		if (data.filters.type) params.set('type', data.filters.type);
		if (data.filters.category) params.set('category', data.filters.category);
		if (data.filters.sort) params.set('sort', data.filters.sort);
		params.set('page', String(p));
		return `/mutual-aid?${params.toString()}`;
	}

	function fetchDestinationCountries(q: string): Promise<AutocompleteItem[]> {
		const allLabel = t('mutual_aid.filter_destination_all', 'All countries');
		const query = q.toLowerCase().trim();
		const allItem: AutocompleteItem = { id: '', label: allLabel };
		if (query === '') {
			return Promise.resolve([allItem, ...(data.countries ?? []).map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }))]);
		}
		const matchesAll = allLabel.toLowerCase().includes(query);
		const filtered = (data.countries ?? []).filter(
			(c) => c.name.toLowerCase().includes(query) || c.geo_code.toLowerCase().includes(query)
		);
		const items = filtered.map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }));
		return Promise.resolve(matchesAll ? [allItem, ...items] : items);
	}
	function fetchDestinationCountriesInitial(): Promise<AutocompleteItem[]> {
		return fetchDestinationCountries('');
	}
	const selectedDestinationLabel = $derived(
		dest ? (data.countries ?? []).find((c) => c.id === dest)?.name ?? null : null
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<link rel="canonical" href={page.url.origin + '/mutual-aid'} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.origin + '/mutual-aid'} />
	{#if currentPage > 1}
		{@const maPrevUrl = (() => { const u = new URL(page.url.href); if (currentPage - 1 <= 1) u.searchParams.delete('page'); else u.searchParams.set('page', String(currentPage - 1)); return u.toString(); })()}
		<link rel="prev" href={maPrevUrl} />
	{/if}
	{#if currentPage < totalPages}
		{@const maNextUrl = (() => { const u = new URL(page.url.href); u.searchParams.set('page', String(currentPage + 1)); return u.toString(); })()}
		<link rel="next" href={maNextUrl} />
	{/if}
</svelte:head>

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 48rem)/2))">
	<h1 class="sr-only">{t('mutual_aid.heading', 'Mutual Aid')}</h1>
	<ListPageBar>
		<UrlTabs
			{tabs}
			value={activeTab}
			onValueChange={(v) => selectType(v)}
			class="!border-b-0 !bg-transparent"
		/>
		{#snippet actions()}
			<FilterToggle bind:show={showFilters} active={hasFilters} controls="mutual-aid-filter-panel" />
			{#if data.authed}
				<PageAction
					href="/mutual-aid/new?type=offer"
					icon={Plus}
					label={t('mutual_aid.post_offer', 'Post Offer')}
					secondary
					fabIndex={1}
				/>
				<PageAction
					href="/mutual-aid/new?type=need"
					icon={Plus}
					label={t('mutual_aid.post_request', 'Post Request')}
				/>
			{:else}
				<PageAction
					href="/login?redirect=/mutual-aid/new"
					icon={Plus}
					label={t('mutual_aid.post_request', 'Post Request')}
				/>
			{/if}
		{/snippet}
	</ListPageBar>

	<!-- Content -->
	<div class="mx-auto w-full max-w-3xl p-4">
		{#if showFilters}
			<!-- Filter panel -->
			<div id="mutual-aid-filter-panel" class="mb-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
		<form onsubmit={applyFilters}>
		<div class="flex flex-wrap items-center gap-3">
			<div class="w-52">
				<Autocomplete
					placeholder={t('mutual_aid.filter_destination', 'Destination')}
					ariaLabel={t('mutual_aid.filter_destination', 'Destination')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={fetchDestinationCountries}
					initialFetcher={fetchDestinationCountriesInitial}
					selectedId={dest || null}
					selectedLabel={selectedDestinationLabel}
					onSelect={(item) => { dest = item.id; applyFilters(); }}
					onClear={() => { dest = ''; applyFilters(); }}
				/>
			</div>

			<div class="w-52">
				<Autocomplete
					placeholder={t('mutual_aid.filter_category', 'Category')}
					ariaLabel={t('mutual_aid.filter_category', 'Category')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={categoryFetcher}
					initialFetcher={categoryFetcher}
					selectedId={category || null}
					selectedLabel={selectedCategoryLabel}
					onSelect={(item) => { category = item.id; applyFilters(); }}
					onClear={() => { category = ''; applyFilters(); }}
				/>
			</div>

			<div class="flex items-center gap-2">
				{#if hasFilters}
					<button
						type="button"
						onclick={clearFilters}
						class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						<X size={14} /> {t('common.clear', 'Clear')}
					</button>
				{/if}
			</div>
		</div>
	</form>
			</div>
		{/if}

		{#if data.loadError}
			<EmptyState
				icon={HandHeart}
				title={t('mutual_aid.load_error', 'Could not load posts')}
				body={t('mutual_aid.load_error_body', 'Please try again later.')}
			/>
		{:else if posts.length === 0}
			<EmptyState
				icon={HandHeart}
				title={t('mutual_aid.empty_title', 'No mutual aid posts yet')}
				body={t('mutual_aid.empty_body', 'Be the first to post a request or offer.')}
				action={data.authed
					? { label: t('mutual_aid.post_request', 'Post Request'), href: '/mutual-aid/new' }
					: { label: t('common.login', 'Log in'), href: '/login?redirect=/mutual-aid/new' }}
			/>
		{:else}
			<div class="space-y-3">
				{#each posts as post (post.id)}
					<a
						href="/mutual-aid/{post.id}"
						class="block rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-primary)]/40"
					>
						<div class="flex flex-wrap items-center gap-2">
							<span class={postTypeBadgeClass(post.post_type as MutualAidPostType)}>
								{postTypeLabel(post.post_type as MutualAidPostType)}
							</span>
							{#if post.category}
								<span class="inline-flex items-center rounded-full border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs font-medium text-[var(--color-text-subtle)]">
									{categoryLabel(post.category as MutualAidCategory)}
								</span>
							{/if}
							<span class={statusBadgeClass(post.status)}>
								{statusLabel(post.status)}
							</span>
						</div>

						<h2 class="mt-2 text-base font-semibold text-[var(--color-text)]">
							{post.title}
						</h2>
						<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
							{excerpt(post.description ?? '')}
						</p>

						<div class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-[var(--color-muted)]">
							{#if post.destination_space_name || post.city_space_name}
								<span class="inline-flex items-center gap-1">
									<MapPin size={12} />
									{[post.city_space_name, post.destination_space_name].filter(Boolean).join(', ')}
								</span>
							{/if}
							{#if post.expires_at}
								<span class="inline-flex items-center gap-1">
									<Clock size={12} />
									{t('mutual_aid.expires', 'Expires')} {relativeTime(post.expires_at, new Date(), i18n.locale)}
								</span>
							{/if}
							<span class="text-[var(--color-text-subtle)]">
								{post.author_name}
							</span>
							<span>·</span>
							<span>{relativeTime(post.created_at, new Date(), i18n.locale)}</span>
						</div>
					</a>
				{/each}
			</div>

			<!-- Pagination -->
			{#if totalPages > 1}
				<nav class="mt-6 flex items-center justify-center gap-2" aria-label={t('common.pagination', 'Pagination')}>
					{#if currentPage > 1}
						<a
							href={pageHref(currentPage - 1)}
							class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							{t('common.prev', 'Previous')}
						</a>
					{/if}
					<span class="text-sm text-[var(--color-muted)]">
						{currentPage} / {totalPages}
					</span>
					{#if currentPage < totalPages}
						<a
							href={pageHref(currentPage + 1)}
							class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							{t('common.next', 'Next')}
						</a>
					{/if}
				</nav>
			{/if}
		{/if}
	</div>
</div>

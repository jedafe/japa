// Ref: japaconnect-engineering/sections/85-mutual-aid-board.md
<script lang="ts">
	import { t } from '$lib/i18n';
	import { i18n } from '$lib/i18n';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { HandHeart, Plus, Search, MapPin, Clock, X } from 'lucide-svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { relativeTime } from '$lib/feed/time';
	import type { MutualAidPost, MutualAidPostType, MutualAidCategory } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const posts = $derived(data.posts);
	const meta = $derived(data.meta);
	const totalPages = $derived(meta?.total_pages ?? 1);
	const currentPage = $derived(data.filters.page);

	// Filter bar state — seeded from the server-provided filter values.
	let dest = $state(data.filters.dest);
	let city = $state(data.filters.city);
	let type = $state(data.filters.type);
	let category = $state(data.filters.category);
	let sort = $state(data.filters.sort);

	// Re-seed when the server payload changes (e.g. browser back/forward).
	$effect(() => {
		dest = data.filters.dest;
		city = data.filters.city;
		type = data.filters.type;
		category = data.filters.category;
		sort = data.filters.sort;
	});

	const postTypeOptions: { value: MutualAidPostType | ''; label: string }[] = [
		{ value: '', label: 'All types' },
		{ value: 'need', label: 'Need' },
		{ value: 'offer', label: 'Offer' },
		{ value: 'skill_swap', label: 'Skill swap' }
	];

	const categoryOptions: { value: MutualAidCategory | ''; label: string }[] = [
		{ value: '', label: 'All categories' },
		{ value: 'housing', label: 'Housing' },
		{ value: 'transport', label: 'Transport' },
		{ value: 'documents', label: 'Documents' },
		{ value: 'job_search', label: 'Job search' },
		{ value: 'language', label: 'Language' },
		{ value: 'social', label: 'Social' },
		{ value: 'other', label: 'Other' }
	];

	const sortOptions = [
		{ value: 'latest', label: 'Latest' },
		{ value: 'nearest', label: 'Nearest' }
	];

	const title = $derived(t('mutual_aid.list_title', 'Mutual Aid — JapaConnect'));
	const description = $derived(
		t(
			'mutual_aid.list_description',
			'Community help requests and offers for the Nigerian diaspora.'
		)
	);

	function applyFilters(e: SubmitEvent) {
		e.preventDefault();
		const params = new URLSearchParams();
		if (dest.trim()) params.set('dest', dest.trim());
		if (city.trim()) params.set('city', city.trim());
		if (type) params.set('type', type);
		if (category) params.set('category', category);
		if (sort) params.set('sort', sort);
		const qs = params.toString();
		goto(`/mutual-aid${qs ? `?${qs}` : ''}`);
	}

	function clearFilters() {
		dest = '';
		city = '';
		type = '';
		category = '';
		sort = 'latest';
		goto('/mutual-aid');
	}

	const hasFilters = $derived(!!dest || !!city || !!type || !!category);

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
		if (dest.trim()) params.set('dest', dest.trim());
		if (city.trim()) params.set('city', city.trim());
		if (type) params.set('type', type);
		if (category) params.set('category', category);
		if (sort) params.set('sort', sort);
		params.set('page', String(p));
		return `/mutual-aid?${params.toString()}`;
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<!-- Header -->
	<div class="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<HandHeart size={18} class="text-[var(--color-primary)]" />
			{t('mutual_aid.heading', 'Mutual Aid')}
		</h1>
		<div class="flex items-center gap-2">
			{#if data.authed}
				<a
					href="/mutual-aid/new?type=offer"
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-green-500/30 bg-green-500/10 px-3 py-1.5 text-xs font-medium text-green-600 transition-colors hover:bg-green-500/20"
				>
					<Plus size={14} /> {t('mutual_aid.post_offer', 'Post Offer')}
				</a>
				<a
					href="/mutual-aid/new?type=need"
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
				>
					<Plus size={14} /> {t('mutual_aid.post_request', 'Post Request')}
				</a>
			{:else}
				<a
					href="/login?redirect=/mutual-aid/new"
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
				>
					<Plus size={14} /> {t('mutual_aid.post_request', 'Post Request')}
				</a>
			{/if}
		</div>
	</div>

	<!-- Filter bar -->
	<form onsubmit={applyFilters} class="border-b border-[var(--color-border)] px-4 py-3">
		<div class="flex flex-wrap items-end gap-3">
			<div class="flex flex-col gap-1">
				<label for="filter-dest" class="text-xs font-medium text-[var(--color-text-subtle)]">
					{t('mutual_aid.filter_destination', 'Destination')}
				</label>
				<input
					id="filter-dest"
					type="text"
					bind:value={dest}
					placeholder={t('mutual_aid.filter_destination_placeholder', 'Country')}
					class="w-40 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
				/>
			</div>

			<div class="flex flex-col gap-1">
				<label for="filter-city" class="text-xs font-medium text-[var(--color-text-subtle)]">
					{t('mutual_aid.filter_city', 'City')}
				</label>
				<input
					id="filter-city"
					type="text"
					bind:value={city}
					placeholder={t('mutual_aid.filter_city_placeholder', 'City')}
					class="w-32 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
				/>
			</div>

			<div class="flex flex-col gap-1">
				<label for="filter-type" class="text-xs font-medium text-[var(--color-text-subtle)]">
					{t('mutual_aid.filter_type', 'Type')}
				</label>
				<select
					id="filter-type"
					bind:value={type}
					class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
				>
					{#each postTypeOptions as opt (opt.value)}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
			</div>

			<div class="flex flex-col gap-1">
				<label for="filter-category" class="text-xs font-medium text-[var(--color-text-subtle)]">
					{t('mutual_aid.filter_category', 'Category')}
				</label>
				<select
					id="filter-category"
					bind:value={category}
					class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
				>
					{#each categoryOptions as opt (opt.value)}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
			</div>

			<div class="flex flex-col gap-1">
				<label for="filter-sort" class="text-xs font-medium text-[var(--color-text-subtle)]">
					{t('mutual_aid.filter_sort', 'Sort')}
				</label>
				<select
					id="filter-sort"
					bind:value={sort}
					class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
				>
					{#each sortOptions as opt (opt.value)}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
			</div>

			<div class="flex items-center gap-2">
				<button
					type="submit"
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
				>
					<Search size={14} /> {t('common.search', 'Search')}
				</button>
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

	<!-- Content -->
	<div class="p-4">
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

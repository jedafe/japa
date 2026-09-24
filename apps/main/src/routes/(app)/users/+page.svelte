<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t } from '$lib/i18n';
	import { Users, Search, ShieldX, ChevronLeft, ChevronRight } from 'lucide-svelte';
	import { EmptyState, FollowButton, SearchInput, DirectoryCard, type SearchSuggestion } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import type { PageData } from './$types';
import { safeJsonLdScript } from '$lib/utils/safeHtml';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('users.title', 'People — JapaConnect'));
	const description = $derived(
		t('users.description', 'Discover people on JapaConnect — creators, professionals, and communities.')
	);

	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'CollectionPage',
			name: 'People',
			description,
			url: page.url.href
		})
	);

	let query = $state(untrack(() => data.q));
	let selectedProfileType = $state(untrack(() => data.profileType));
	const searchPlaceholder = $derived(t('users.search_placeholder', 'Search people…'));

	// User-account profile_type enum — fixed backend constraint per §unified-tiers.
	// These are the user-account tiers (regular / pro / business).
	const profileTypes = $derived([
		{ value: '', label: t('users.profile_type_all', 'All') },
		{ value: 'regular', label: t('users.profile_type_regular', 'Regular') },
		{ value: 'pro', label: t('users.profile_type_pro', 'Pro') },
		{ value: 'business', label: t('users.profile_type_business', 'Business') }
	]);

	function buildHref(pageNum: number): string {
		const params = new URLSearchParams();
		if (data.q) params.set('q', data.q);
		if (data.profileType) params.set('profile_type', data.profileType);
		if (pageNum > 0) params.set('page', String(pageNum));
		const qs = params.toString();
		return qs ? `/users?${qs}` : '/users';
	}

	function onFilterChange() {
		const params = new URLSearchParams();
		if (query.trim()) params.set('q', query.trim());
		if (selectedProfileType) params.set('profile_type', selectedProfileType);
		const qs = params.toString();
		goto(qs ? `/users?${qs}` : '/users');
	}

	async function userSearchFetcher(q: string): Promise<SearchSuggestion[]> {
		const res = await api.get<{ data: { results: { type: string; id: string; title: string; excerpt?: string; url: string }[] } }>(
			`/search?q=${encodeURIComponent(q)}&type=user&limit=8`
		);
		if (res.error || !res.data) return [];
		return (res.data.data?.results ?? []).map((r) => ({
			id: r.id,
			title: r.title,
			url: r.url,
			excerpt: r.excerpt,
			type: r.type
		}));
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
	{@html safeJsonLdScript(jsonLd)}
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<!-- Header + search + filters -->
	<div class="border-b border-[var(--color-border)] px-4 py-4">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Users size={20} /> {t('users.heading', 'People')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
			{t('users.subtitle', 'Follow people to see their posts in your feed.')}
		</p>

		<form class="mt-3" action="/users" method="get" role="search">
			<SearchInput
				bind:value={query}
				placeholder={searchPlaceholder}
				ariaLabel={t('users.heading', 'People')}
				fetcher={userSearchFetcher}
			/>
			<!-- Hidden fields to preserve filters on submit -->
			{#if selectedProfileType}
				<input type="hidden" name="profile_type" value={selectedProfileType} />
			{/if}
		</form>

		<!-- Profile type filter chips -->
		<div class="mt-3 flex flex-wrap gap-2">
			{#each profileTypes as pt (pt.value)}
				<button
					type="button"
					onclick={() => { selectedProfileType = pt.value; onFilterChange(); }}
					class="rounded-full border px-3 py-1 text-xs transition-colors {selectedProfileType === pt.value
						? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
						: 'border-[var(--color-border)] text-[var(--color-muted)] hover:border-[var(--color-primary)]/40'}"
				>
					{pt.label}
				</button>
			{/each}
		</div>
	</div>

	<div class="p-4">
		{#if data.forbidden}
			<div class="flex flex-col items-center gap-3 py-16 text-center">
				<ShieldX size={40} class="text-[var(--color-muted)]" />
				<p class="text-sm font-medium text-[var(--color-text)]">
					{t('users.forbidden', 'You do not have permission to view this directory.')}
				</p>
			</div>
		{:else if data.loadError}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">
				{t('users.load_error', 'Failed to load people. Please try again later.')}
			</p>
		{:else if data.users.length === 0}
			<EmptyState icon={Users} title={t('users.empty', 'No people yet.')} />
		{:else}
			<p class="mb-3 text-xs text-[var(--color-muted)]">
				{data.total} {data.total === 1 ? t('users.results_one', 'person') : t('users.results', 'people')}
			</p>
			<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
				{#each data.users as user (user.id)}
					<DirectoryCard
						href="/u/{user.username}"
						title={user.display_name ?? user.username}
						subtitle="@{user.username}"
						description={user.bio}
						imageUrl={user.avatar_url}
						fallback={(user.display_name ?? user.username).charAt(0).toUpperCase()}
						hueSeed={user.id}
						round
						verified={user.profile_type === 'business' || user.profile_type === 'pro'}
						meta={[{ icon: Users, text: (user.follower_count === 1 ? t('profile.followers_one', '{n} follower') : t('profile.followers', '{n} followers')).replace('{n}', String(user.follower_count)) }]}
						entityType="user"
						entityId={user.id}
						entitySlug={user.username}
						entityCount={user.follower_count}
					>
						{#snippet actions()}
							{#if page.data.user && page.data.user.id !== user.id}
								<FollowButton entityType="user" entityId={user.id} compact />
							{/if}
						{/snippet}
					</DirectoryCard>
				{/each}
			</div>

			<!-- Pagination -->
			{#if data.totalPages > 1}
				<div class="mt-6 flex items-center justify-center gap-4">
					<a
						href={buildHref(data.page - 1)}
						class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] {data.page === 0 ? 'pointer-events-none opacity-40' : ''}"
					>
						<ChevronLeft size={16} /> {t('common.prev', 'Prev')}
					</a>
					<span class="text-xs text-[var(--color-muted)]">
						{data.page + 1} / {data.totalPages}
					</span>
					<a
						href={buildHref(data.page + 1)}
						class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] {data.page >= data.totalPages - 1 ? 'pointer-events-none opacity-40' : ''}"
					>
						{t('common.next', 'Next')} <ChevronRight size={16} />
					</a>
				</div>
			{/if}
		{/if}
	</div>
</div>

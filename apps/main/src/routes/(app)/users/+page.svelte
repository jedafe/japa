<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t } from '$lib/i18n';
	import {
		Users,
		Check,
		ShieldX,
		ChevronRight,
		Search,
		UserPlus,
		Filter,
		Loader2,
		MapPin,
		GraduationCap
	} from 'lucide-svelte';
	import {
		EmptyState,
		EntityHero,
		FollowButton,
		GuideSlideOver,
		ListPageBar,
		Modal,
		SearchInput,
		UrlTabs,
		DirectoryCard,
		type GuideContent,
		type SearchSuggestion,
		type UrlTab,
		type CardMetaItem
	} from '$lib/components/ui';
	import HeroArtUsers from '$lib/components/heroes/HeroArtUsers.svelte';
	import { formatMetric } from '$lib/components/hub/charts/format';
	import { api } from '$lib/api/client';
	import type { PageData } from './$types';
	import type { PublicUser } from '$lib/types';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('users.title', 'People — {site}'));
	const description = $derived(
		t('users.description', 'Discover people on {site} — creators, professionals, and communities.')
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
	let sort = $state(untrack(() => data.sort));
	// Accumulated result list — the SSR page loads the first batch; "Load
	// more" appends subsequent offset pages client-side (no pagination UI).
	let users = $state<PublicUser[]>(untrack(() => data.users));
	let loadingMore = $state(false);
	let loadMoreError = $state(false);
	const searchPlaceholder = $derived(t('users.search_placeholder', 'Search people…'));

	$effect(() => {
		query = data.q;
		selectedProfileType = data.profileType;
		sort = data.sort;
		users = data.users;
		loadMoreError = false;
	});

	// User-account profile_type enum — fixed backend constraint per §unified-tiers.
	// These are the user-account tiers (regular / pro / business).
	const profileTypeTabs = $derived<UrlTab[]>([
		{ value: '', label: t('users.profile_type_all', 'All') },
		{ value: 'regular', label: t('users.profile_type_regular', 'Regular') },
		{ value: 'pro', label: t('users.profile_type_pro', 'Pro') },
		{ value: 'business', label: t('users.profile_type_business', 'Business') }
	]);

	// Sort keys are whitelisted on the backend (GET /users?sort=) — unknown
	// values fall back to 'followers'.
	const sortOptions = $derived<{ value: string; label: string }[]>([
		{ value: 'followers', label: t('users.sort.followers', 'Most followed') },
		{ value: 'newest', label: t('users.sort.newest', 'Newest') },
		{ value: 'oldest', label: t('users.sort.oldest', 'Oldest') },
		{ value: 'name', label: t('users.sort.name', 'Name A–Z') }
	]);

	// Right-side bar controls — each opens a modal rather than taking bar space.
	let sortModalOpen = $state(false);
	let searchModalOpen = $state(false);

	const hasMore = $derived(users.length < data.total);

	function onFilterChange() {
		const params = new URLSearchParams();
		if (query.trim()) params.set('q', query.trim());
		if (selectedProfileType) params.set('profile_type', selectedProfileType);
		if (sort !== 'followers') params.set('sort', sort);
		const qs = params.toString();
		goto(qs ? `/users?${qs}` : '/users', { keepFocus: true });
	}

	// Offset pagination mirrors the server loader (page_size/page_offset);
	// filters come from `data` (post-navigation truth), not the local mirrors.
	async function loadMore() {
		if (loadingMore || !hasMore) return;
		loadingMore = true;
		loadMoreError = false;
		const params = new URLSearchParams();
		params.set('page_size', String(data.pageSize));
		params.set('page_offset', String(users.length));
		if (data.q) params.set('q', data.q);
		if (data.profileType) params.set('profile_type', data.profileType);
		if (data.sort !== 'followers') params.set('sort', data.sort);
		const res = await api.get<PublicUser[]>(`/users?${params.toString()}`);
		if (res.error || !res.data) {
			loadMoreError = true;
		} else {
			const seen = new Set(users.map((u) => u.id));
			users = [...users, ...res.data.filter((u) => !seen.has(u.id))];
		}
		loadingMore = false;
	}

	// User typeahead — GET /search/autocomplete (public, type=user) returns
	// flat suggestions {id, url, username, display_name}; the search index
	// now emits canonical /u/{username} urls (stale /users/ docs are
	// normalised server-side).
	async function userSearchFetcher(q: string): Promise<SearchSuggestion[]> {
		// api.get already unwraps the {data: ...} envelope.
		const res = await api.get<{
			suggestions: { type: string; id: string; url?: string; username?: string; display_name?: string }[];
		}>(`/search/autocomplete?q=${encodeURIComponent(q)}&type=user&limit=8`);
		if (res.error || !res.data) return [];
		return (res.data.suggestions ?? []).map((s) => ({
			id: s.id,
			title: s.display_name || s.username || '',
			url: s.url
				? s.url.replace(/^\/users\//, '/u/')
				: s.username
					? `/u/${s.username}`
					: '/users',
			excerpt: s.username ? `@${s.username}` : undefined,
			type: s.type
		}));
	}

	// "How it works" slide-over — markdown panel + CTA repeater persisted to
	// public system settings (users.guide) via PUT /admin/settings/general.
	let guideOpen = $state(false);
	let guideOverride = $state<GuideContent | null>(null);
	const siteSettings = $derived((page.data.siteSettings ?? {}) as Record<string, unknown>);
	const storedGuide = $derived.by((): Partial<GuideContent> => {
		const raw = siteSettings['users.guide'];
		if (raw && typeof raw === 'object' && !Array.isArray(raw)) return raw as Partial<GuideContent>;
		return {};
	});
	const effectiveGuide = $derived<GuideContent>({
		title: guideOverride?.title ?? storedGuide.title ?? t('common.how_it_works', 'How it works'),
		body: guideOverride?.body ?? storedGuide.body ?? t('users.guide_body', ''),
		ctas: guideOverride?.ctas ?? storedGuide.ctas ?? [{ title: t('guide_panel.read_full_guide', 'Read the full guide'), url: '/guides' }]
	});

	async function saveGuide(v: GuideContent): Promise<string | null> {
		const res = await api.put('/admin/settings/general', { users_guide: v });
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
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
	{@html safeJsonLdScript(jsonLd)}
</svelte:head>

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 48rem)/2))">
	<!-- Header — EntityHero directory panel -->
	<EntityHero
		class="mt-4"
		icon={Users}
		accent="var(--color-primary)"
		heading={t('users.heading', 'People')}
		description={t('users.hero_desc', 'Discover and connect with people in the community — learn, collaborate, and grow together.')}
		crumbs={[
			{ label: t('common.home', 'Home'), href: '/' },
			{ label: t('users.heading', 'People') }
		]}
		stats={[
			{ icon: Users, value: formatMetric(data.total), label: t('users.stats_members', 'Members') }
		]}
	>
		{#snippet art()}
			<HeroArtUsers accent="var(--color-primary)" />
		{/snippet}
		{#snippet actions(c)}
			<div class="grid w-full grid-cols-2 gap-3 sm:flex sm:items-center">
				<a href="#users-results" class="{c.primary} flex-1 justify-center whitespace-nowrap">
					<Search size={15} /> {t('users.find', 'Find people')}
				</a>
				{#if page.data.user}
					<a href="/dashboard" class="{c.secondary} flex-1 justify-center whitespace-nowrap">
						<UserPlus size={15} /> {t('users.invite', 'Invite people')}
					</a>
				{/if}
				<button type="button" onclick={() => (guideOpen = true)} class="{c.ghost} flex-1 justify-center whitespace-nowrap col-span-2 sm:col-span-1">
					{t('common.how_it_works', 'How it works')}
					<ChevronRight size={14} class="rtl:-scale-x-100" />
				</button>
			</div>
		{/snippet}
	</EntityHero>

	<!-- Profile type tabs on the left; Sort + Search buttons on the right —
	     one ListPageBar row (the /org pattern). -->
	<ListPageBar>
		<UrlTabs
			tabs={profileTypeTabs}
			value={selectedProfileType}
			onValueChange={(v) => {
				selectedProfileType = v;
				onFilterChange();
			}}
			class="!border-b-0 !bg-transparent"
		/>
		{#snippet actions()}
			<button
				type="button"
				onclick={() => (sortModalOpen = true)}
				class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
			>
				{t('users.sort.label', 'Sort')}
				{#if sort !== 'followers'}
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
				{#if query.trim()}
					<span class="ms-0.5 h-2 w-2 rounded-full bg-[var(--color-primary)]" aria-hidden="true"></span>
				{/if}
				<Search size={16} />
			</button>
		{/snippet}
	</ListPageBar>

	<!-- Sort picker modal — same whitelisted keys the old dropdown used. -->
	<Modal
		bind:open={sortModalOpen}
		title={t('users.sort.label', 'Sort')}
		icon={Filter}
		size="sm"
	>
		{#snippet children(close)}
			<div role="listbox" aria-label={t('users.sort.label', 'Sort')} class="-m-1">
				{#each sortOptions as opt (opt.value)}
					<button
						type="button"
						role="option"
						aria-selected={sort === opt.value}
						onclick={() => { sort = opt.value; onFilterChange(); close(); }}
						class="flex w-full items-center justify-between gap-2 rounded-[var(--radius)] px-3 py-2.5 text-sm font-medium transition-colors {sort === opt.value
							? 'text-[var(--color-text)]'
							: 'text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]'}"
					>
						{opt.label}
						{#if sort === opt.value}
							<Check size={16} class="text-[var(--color-primary)]" />
						{/if}
					</button>
				{/each}
			</div>
		{/snippet}
	</Modal>

	<!-- Search modal — same GET form as the previous inline bar search. -->
	<Modal bind:open={searchModalOpen} title={t('common.search', 'Search')} icon={Search} size="sm">
		{#snippet children()}
			<form action="/users" method="get" role="search" onsubmit={() => (searchModalOpen = false)}>
				<SearchInput
					bind:value={query}
					placeholder={searchPlaceholder}
					ariaLabel={t('users.heading', 'People')}
					fetcher={userSearchFetcher}
				/>
				<!-- Hidden fields to preserve the active filters on submit -->
				{#if selectedProfileType}
					<input type="hidden" name="profile_type" value={selectedProfileType} />
				{/if}
				{#if sort !== 'followers'}
					<input type="hidden" name="sort" value={sort} />
				{/if}
			</form>
		{/snippet}
	</Modal>

	<div id="users-results" class="mx-auto w-full max-w-3xl p-4">
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
		{:else if users.length === 0}
			<EmptyState icon={Users} title={t('users.empty', 'No people yet.')} />
		{:else}
			<p class="mb-3 text-xs text-[var(--color-muted)]">
				{data.total} {data.total === 1 ? t('users.results_one', 'person') : t('users.results', 'people')}
			</p>
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
				{#each users as user (user.id)}
					{@const tierLabel = t(`users.profile_type_${user.profile_type}`, user.profile_type)}
					<DirectoryCard
						href="/u/{user.username}"
						title={user.display_name ?? user.username}
						subtitle="@{user.username}"
						description={user.bio}
						imageUrl={user.avatar_url}
						fallback={(user.display_name ?? user.username ?? '?').charAt(0).toUpperCase()}
						hueSeed={user.id}
						round
						verified={!!user.verified}
						meta={[
							{ icon: GraduationCap, text: tierLabel },
							...(user.location ? [{ icon: MapPin, text: user.location }] : []),
							{ icon: Users, text: (user.follower_count === 1 ? t('profile.followers_one', '{n} follower') : t('profile.followers', '{n} followers')).replace('{n}', String(user.follower_count)) },
							{ icon: UserPlus, text: (user.following_count === 1 ? t('profile.following_one', '{n} following') : t('profile.following', '{n} following')).replace('{n}', String(user.following_count)) }
						] satisfies CardMetaItem[]}
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

			<!-- Load more — offset paging appended to the SSR batch. -->
			{#if loadMoreError}
				<p class="mt-4 text-center text-sm text-[var(--color-text-subtle)]">
					{t('users.load_error', 'Failed to load people. Please try again later.')}
				</p>
			{/if}
			{#if hasMore}
				<div class="mt-6 flex justify-center">
					<button
						type="button"
						onclick={loadMore}
						disabled={loadingMore}
						aria-busy={loadingMore}
						class="inline-flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:cursor-not-allowed disabled:opacity-50"
					>
						{#if loadingMore}
							<Loader2 size={16} class="animate-spin" />
						{:else}
							{t('common.load_more', 'Load more')}
						{/if}
					</button>
				</div>
			{/if}
		{/if}
	</div>
</div>

<!-- "How it works" slide-over — GuideSlideOver; admins edit inline (users.guide). -->
<GuideSlideOver
	bind:open={guideOpen}
	title={effectiveGuide.title}
	body={effectiveGuide.body}
	ctas={effectiveGuide.ctas}
	canManage={data.canManage}
	onSave={saveGuide}
/>

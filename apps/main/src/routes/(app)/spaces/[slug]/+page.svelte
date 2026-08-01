<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Rss, MessageSquare, Users, Info, PenSquare, Search, BarChart3 } from 'lucide-svelte';
	import SetSidebar from '$lib/components/shell/SetSidebar.svelte';
	import ProfileTabs from '$lib/components/profile/ProfileTabs.svelte';
	import SpaceSidebar from '$lib/components/profile/SpaceSidebar.svelte';
	import ForumList from '$lib/components/forum/ForumList.svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import type { SpaceContentItem } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const space = $derived(data.space);
	const authed = $derived(!!page.data.user);
	const activeTab = $derived(page.url.searchParams.get('tab') ?? 'feed');

	// Hydrate the Join button's initial state: the caller is a member if the
	// current space appears in their joined-spaces list (page.data.userSpaces,
	// fetched SSR via GET /me/spaces in the root layout). This is the
	// authoritative membership source; the toggle itself also reads the
	// server response for the post-action state.
	const initialJoined = $derived(
		authed && !!page.data.userSpaces?.some((s: { id: string; slug: string }) => s.id === space.id || s.slug === space.slug)
	);
	const initiallyFollowing = $derived(data.initiallyFollowing ?? false);

	const analytics = $derived(data.analytics as {
		total_views?: number;
		total_posts?: number;
		total_members?: number;
		total_followers?: number;
		posts_last_30d?: number;
	} | null);

	const tabs = $derived([
		{ key: 'feed', labelKey: 'space.tab.feed', href: `?tab=feed` },
		{ key: 'forum', labelKey: 'space.tab.forum', href: `?tab=forum` },
		{ key: 'search', labelKey: 'space.tab.search', href: `?tab=search` },
		{ key: 'members', labelKey: 'space.tab.members', href: `?tab=members` },
		{ key: 'about', labelKey: 'space.tab.about', href: `?tab=about` },
		// Analytics dashboard — space-admin only per JEOS §215 [auth,
		// space-admin]. The server only returns analytics data to the space
		// owner/moderators (403 → null otherwise), so the tab is hidden for
		// guests and non-admin members.
		...(authed && analytics
			? [{ key: 'analytics', labelKey: 'space.tab.analytics', href: `?tab=analytics` }]
			: [])
	]);

	// Search tab state (Phase 08.09).
	let searchQuery = $state('');
	let searchResults = $state<SpaceContentItem[]>([]);
	let searching = $state(false);

	async function runSearch() {
		if (!searchQuery.trim()) {
			searchResults = [];
			return;
		}
		searching = true;
		const res = await api.get<SpaceContentItem[]>(
			`/spaces/${space.slug}/content?q=${encodeURIComponent(searchQuery.trim())}&page_size=20`
		);
		searchResults = res.data ?? [];
		searching = false;
	}

	const title = $derived(`${space.name} — ${t('app.name')}`);
	const description = $derived(space.description ?? `${space.name} on ${t('app.name')}`);

	// Client-side load-more for the space feed (cursor-less page_offset paging).
	let feedItems = $state<SpaceContentItem[]>(untrack(() => data.content));
	let feedLoading = $state(false);
	let feedEnd = $derived(data.content.length < data.pageSize);

	$effect(() => {
		// Re-seed on SSR payload change (slug change).
		feedItems = data.content;
	});

	async function loadMoreFeed() {
		if (feedLoading || feedEnd) return;
		feedLoading = true;
		const res = await api.get<SpaceContentItem[]>(
			`/spaces/${space.slug}/content?page_size=${data.pageSize}&page_offset=${feedItems.length}`
		);
		if (res.data) {
			feedItems = [...feedItems, ...res.data];
			if (res.data.length < data.pageSize) feedEnd = true;
		}
		feedLoading = false;
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<SetSidebar>
	<SpaceSidebar {space} rules={data.rules} {authed} {initialJoined} {initiallyFollowing} />
</SetSidebar>

<!-- Space header -->
<div class="border-b border-[var(--color-border)] px-4 py-4">
	<div class="mx-auto flex w-full max-w-2xl items-start gap-3">
		<span
			class="grid h-14 w-14 shrink-0 place-items-center rounded-[var(--radius-card)] bg-[var(--color-surface-subtle)] text-lg font-semibold text-[var(--color-text-subtle)]"
		>
			{space.name.charAt(0).toUpperCase()}
		</span>
		<div class="min-w-0 flex-1">
			<h1 class="truncate text-lg font-semibold text-[var(--color-text)]">{space.name}</h1>
			<p class="text-xs capitalize text-[var(--color-muted)]">{space.level} · {space.visibility}</p>
			{#if space.description}
				<p class="mt-1 line-clamp-2 text-sm text-[var(--color-text-subtle)]">{space.description}</p>
			{/if}
		</div>
		{#if authed}
			<a
				href="/create?type=forum&space={space.slug}"
				class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				<PenSquare size={14} /> {t('space.post', 'Post')}
			</a>
		{/if}
	</div>
</div>

<ProfileTabs {tabs} active={activeTab} />

<div class="mx-auto w-full max-w-2xl">
	<div class="p-3">
		{#if activeTab === 'feed'}
			{#if feedItems.length === 0}
				<EmptyState icon={Rss} title={t('space.feed_empty', 'No posts yet')} />
			{:else}
				<div class="divide-y divide-[var(--color-border)] overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
					{#each feedItems as item (item.id)}
						<a href="/content/{item.slug}" class="block p-4 transition-colors hover:bg-[var(--color-surface-subtle)]">
							<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
								<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 font-medium text-[var(--color-text-subtle)]">{item.content_type}</span>
								{#if item.is_pinned}
									<span class="font-medium text-[var(--color-primary)]">{t('space.pinned', 'Pinned')}</span>
								{/if}
								<span>· {item.read_time_minutes} {t('feed.min_read')}</span>
							</div>
							<h3 class="mt-1.5 text-sm font-medium text-[var(--color-text)]">{item.title}</h3>
							<div class="mt-2 flex items-center gap-4 text-xs text-[var(--color-muted)]">
								<span>{item.view_count} {t('profile.views', 'views')}</span>
								<span>{item.comment_count} {t('profile.comments', 'comments')}</span>
							</div>
						</a>
					{/each}
				</div>
				{#if !feedEnd}
					<button
						type="button"
						onclick={loadMoreFeed}
						disabled={feedLoading}
						class="mx-auto mt-3 block rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					>
						{feedLoading ? t('common.loading') : t('feed.load_more', 'Load more')}
					</button>
				{/if}
			{/if}
		{:else if activeTab === 'forum'}
			{#if data.forumThreads.length === 0}
				<EmptyState icon={MessageSquare} title={t('space.forum_empty', 'No discussions yet')} />
			{:else}
				<ForumList threads={data.forumThreads} pageSize={data.pageSize} loadError={false} />
			{/if}
		{:else if activeTab === 'members'}
			{#if data.members.length === 0}
				<EmptyState icon={Users} title={t('space.members_empty', 'No members yet')} />
			{:else}
				<ul class="space-y-2">
					{#each data.members as member (member.user_id)}
						<li>
							<a
								href="/u/{member.username}"
								class="flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-colors hover:bg-[var(--color-surface-subtle)]"
							>
								{#if member.avatar_url}
									<img src={member.avatar_url} alt="" class="h-10 w-10 rounded-full object-cover" />
								{:else}
									<span class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]">
										{(member.display_name ?? member.username).charAt(0).toUpperCase()}
									</span>
								{/if}
								<span class="min-w-0 flex-1">
									<span class="block truncate text-sm font-medium text-[var(--color-text)]">{member.display_name ?? member.username}</span>
									<span class="block text-xs text-[var(--color-muted)]">@{member.username}</span>
								</span>
							</a>
						</li>
					{/each}
				</ul>
			{/if}
		{:else if activeTab === 'about'}
			<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				{#if space.description}
					<p class="text-sm leading-relaxed text-[var(--color-text)]">{space.description}</p>
				{:else}
					<EmptyState icon={Info} title={t('space.no_about', 'No description yet')} />
				{/if}
				{#if space.wiki_content}
					<div class="mt-4 border-t border-[var(--color-border)] pt-4">
						<h2 class="mb-2 text-sm font-medium text-[var(--color-text)]">{t('space.wiki', 'Wiki')}</h2>
						<p class="text-sm leading-relaxed text-[var(--color-text-subtle)]">{space.wiki_content}</p>
					</div>
				{/if}
			</section>
		{:else if activeTab === 'search'}
			<!-- Search tab (Phase 08.09) -->
			<form class="mb-3" onsubmit={(e) => { e.preventDefault(); runSearch(); }}>
				<div class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 focus-within:border-[var(--color-primary)]">
					<Search size={16} class="text-[var(--color-muted)]" />
					<input
						type="search"
						bind:value={searchQuery}
						placeholder={t('space.search_placeholder', 'Search this space…')}
						class="w-full bg-transparent text-sm text-[var(--color-text)] outline-none placeholder:text-[var(--color-muted)]"
					/>
				</div>
			</form>
			{#if searching}
				<p class="py-8 text-center text-sm text-[var(--color-muted)]">{t('space.searching', 'Searching…')}</p>
			{:else if searchQuery.trim() && searchResults.length === 0}
				<EmptyState icon={Search} title={t('space.no_search_results', 'No results found')} />
			{:else if searchResults.length > 0}
				<div class="divide-y divide-[var(--color-border)] overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
					{#each searchResults as item (item.id)}
						<a href="/content/{item.slug}" class="block p-4 transition-colors hover:bg-[var(--color-surface-subtle)]">
							<h3 class="text-sm font-medium text-[var(--color-text)]">{item.title}</h3>
							<p class="mt-1 text-xs text-[var(--color-muted)]">{item.content_type}</p>
						</a>
					{/each}
				</div>
			{/if}
		{:else if activeTab === 'analytics'}
			<!-- Analytics tab (Phase 08.10) -->
			{#if analytics}
				<div class="grid grid-cols-2 gap-3 sm:grid-cols-3">
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
							<BarChart3 size={14} /> {t('space.analytics.views', 'Total views')}
						</div>
						<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{analytics.total_views ?? 0}</p>
					</div>
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
							<Rss size={14} /> {t('space.analytics.posts', 'Total posts')}
						</div>
						<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{analytics.total_posts ?? 0}</p>
					</div>
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
							<Users size={14} /> {t('space.analytics.members', 'Members')}
						</div>
						<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{analytics.total_members ?? 0}</p>
					</div>
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
							<Users size={14} /> {t('space.analytics.followers', 'Followers')}
						</div>
						<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{analytics.total_followers ?? 0}</p>
					</div>
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
							<Rss size={14} /> {t('space.analytics.posts_30d', 'Posts (30d)')}
						</div>
						<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{analytics.posts_last_30d ?? 0}</p>
					</div>
				</div>
			{:else}
				<EmptyState icon={BarChart3} title={t('space.analytics.unavailable', 'Analytics unavailable')} />
			{/if}
		{/if}
	</div>
</div>

<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto, invalidate } from '$app/navigation';
	import { api } from '$lib/api/client';
	import type { SpaceType } from '$lib/types';
	import {
		toggleFollow,
		leaveSpace,
		unfollowTag,
		unblockUser,
		unmuteUser,
		unmuteSpace
	} from '$lib/api/hub';
	import type { FollowableType } from '$lib/api/hub';
	import { optimisticAction } from '$lib/components/hub/optimisticAction';
	import MetricCard from '$lib/components/hub/MetricCard.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import { Users, ChevronLeft, ChevronRight, Search } from 'lucide-svelte';

	// The backend returns a uniform FollowingInfo shape for /me/following and
	// /me/followed-tags: { id, entity_type, entity_id, entity_name, entity_slug,
	// entity_display_name, entity_avatar_url, is_following, created_at }.
	// The dashboard maps these to type-specific display fields.
	interface FollowedEntity {
		id: string;
		entity_type: string;
		entity_id: string;
		entity_name?: string;
		entity_slug?: string;
		entity_display_name?: string;
		entity_avatar_url?: string;
		is_following?: boolean;
		created_at?: string;
	}

	interface FollowerInfo {
		id: string;
		follower_id: string;
		username?: string;
		display_name?: string;
		avatar_url?: string;
		profile_type?: string;
		is_following?: boolean;
		created_at?: string;
	}

	interface Space {
		id: string;
		slug?: string;
		name?: string;
		is_primary?: boolean;
		level?: string;
		space_type_id?: string;
	}

	interface BlockedUser {
		blocked_id: string;
		username?: string;
		display_name?: string;
		avatar_url?: string;
		created_at?: string;
	}

	interface MutedUser {
		muted_user_id: string;
		username?: string;
		display_name?: string;
		avatar_url?: string;
		created_at?: string;
	}

	interface MutedSpace {
		space_id: string;
		space_name?: string;
		space_slug?: string;
		is_primary?: boolean;
		created_at?: string;
		level?: string;
		space_type_id?: string;
	}

	interface RelationshipCounts {
		following?: number;
		followers?: number;
		spaces?: number;
		tags?: number;
		blocked?: number;
		mutedUsers?: number;
		mutedSpaces?: number;
	}

	interface RelationshipsData {
		following: FollowedEntity[];
		followers: FollowerInfo[];
		spaces: Space[];
		tags: FollowedEntity[];
		blocked: BlockedUser[];
		mutedUsers: MutedUser[];
		mutedSpaces: MutedSpace[];
		counts?: RelationshipCounts | null;
		hasMore?: Record<string, boolean> | null;
		page?: number;
		pageSize?: number;
		q?: string;
		type?: string;
		spaceType?: string;
		level?: string;
	}

	let { data }: { data: unknown } = $props();
	const rel = $derived(data as RelationshipsData | null);

	// Optimistic removals are tracked in a single keyed record. Display lists are
	// derived from the server data so invalidation always resyncs correctly.
	let optimisticRemoved = $state<Record<string, boolean>>({});

	// Per-tab client-side search text. The active tab's value is used for
	// local filtering; for the server-search tabs (spaces, muted-spaces) it
	// is also sync'd to the URL `q` parameter after a short debounce.
	let tabSearch = $state<Record<string, string>>({});

	// Available space types for the space type filter (loaded from the API).
	let spaceTypes = $state<{ id: string; name: string }[]>([]);

	$effect(() => {
		// Load space types once for the space tab filter.
		api.get<SpaceType[]>('/space-types').then((res) => {
			if (res.data) {
				spaceTypes = res.data.map((st) => ({ id: st.id, name: st.name }));
			}
		});
	});

	const activeTab = $derived(page.url.searchParams.get('tab') ?? 'following');
	const currentPage = $derived(Math.max(1, parseInt(page.url.searchParams.get('page') ?? '1', 10) || 1));
	const pageSize = $derived(rel?.pageSize ?? 25);
	const serverSearch = $derived(page.url.searchParams.get('q') ?? '');
	const typeFilter = $derived(page.url.searchParams.get('type') ?? '');
	const spaceTypeFilter = $derived(page.url.searchParams.get('space_type') ?? '');
	const levelFilter = $derived(page.url.searchParams.get('level') ?? '');

	const tabs: { key: string; labelKey: string }[] = [
		{ key: 'following', labelKey: 'dashboard.relationships.tab_following' },
		{ key: 'followers', labelKey: 'dashboard.relationships.tab_followers' },
		{ key: 'spaces', labelKey: 'dashboard.relationships.tab_spaces' },
		{ key: 'tags', labelKey: 'dashboard.relationships.tab_tags' },
		{ key: 'blocked', labelKey: 'dashboard.relationships.tab_blocked' },
		{ key: 'muted-users', labelKey: 'dashboard.relationships.tab_muted_users' },
		{ key: 'muted-spaces', labelKey: 'dashboard.relationships.tab_muted_spaces' }
	];

	const followTypeOptions = [
		{ value: '', label: t('dashboard.relationships.all_types', 'All') },
		{ value: 'user', label: t('dashboard.relationships.type_user', 'People') },
		{ value: 'space', label: t('dashboard.relationships.type_space', 'Spaces') },
		{ value: 'tag', label: t('dashboard.relationships.type_tag', 'Tags') },
		{ value: 'organization', label: t('dashboard.relationships.type_organization', 'Organizations') },
		// /me/following?type=source is supported by the backend (mapTypeFilter)
		// and sources are followable via /sources/{slug}/follow.
		{ value: 'source', label: t('dashboard.relationships.type_source', 'Sources') }
	];

	const spaceLevelOptions = [
		{ value: '', label: t('dashboard.relationships.all_levels', 'All levels') },
		{ value: 'country', label: t('dashboard.relationships.level_country', 'Country') },
		{ value: 'state', label: t('dashboard.relationships.level_state', 'State') },
		{ value: 'city', label: t('dashboard.relationships.level_city', 'City') },
		{ value: 'custom', label: t('dashboard.relationships.level_custom', 'Community') },
		{ value: 'global', label: t('dashboard.relationships.level_global', 'Global') }
	];

	const pageSizeOptions = [
		{ value: '10', label: '10' },
		{ value: '25', label: '25' },
		{ value: '50', label: '50' }
	];

	function removedKey(tab: string, id: string) {
		return `${tab}:${id}`;
	}

	function isServerSearchTab(tab: string) {
		return tab === 'spaces' || tab === 'muted-spaces';
	}

	function setSearchParams(updates: Record<string, string | null>) {
		const url = new URL(page.url);
		for (const [k, v] of Object.entries(updates)) {
			if (v === null || v === '') {
				url.searchParams.delete(k);
			} else {
				url.searchParams.set(k, v);
			}
		}
		// Reset to page 1 when filters change.
		if (!('page' in updates)) {
			url.searchParams.delete('page');
		}
		goto(`${url.pathname}${url.search}`, { keepFocus: true, noScroll: true });
	}

	let searchTimer: ReturnType<typeof setTimeout> | null = null;

	function handleSearchInput(value: string) {
		if (searchTimer) clearTimeout(searchTimer);
		searchTimer = setTimeout(() => {
			setSearchParams({ q: value });
		}, 300);
	}

	function onSearchInput(value: string) {
		tabSearch[activeTab] = value;
		if (isServerSearchTab(activeTab)) {
			handleSearchInput(value);
		}
	}

	const searchInputValue = $derived(
		isServerSearchTab(activeTab)
			? (tabSearch[activeTab] ?? serverSearch)
			: (tabSearch[activeTab] ?? '')
	);

	function switchTab(key: string) {
		setSearchParams({ tab: key, page: null });
	}

	function goPage(delta: number) {
		const next = Math.max(1, currentPage + delta);
		setSearchParams({ page: String(next) });
	}

	const dateFormatter = new Intl.DateTimeFormat(undefined, {
		year: 'numeric',
		month: 'short',
		day: 'numeric'
	});

	function formatDate(iso: string | undefined) {
		if (!iso) return '';
		try {
			return dateFormatter.format(new Date(iso));
		} catch {
			return '';
		}
	}

	function initials(name: string | undefined | null) {
		return name?.charAt(0)?.toUpperCase() ?? '?';
	}

	function displayName(item: { entity_name?: string; entity_display_name?: string }) {
		return item.entity_display_name || item.entity_name || '';
	}

	function matchesSearch(name: string | undefined | null, query: string) {
		if (!query) return true;
		if (!name) return false;
		return name.toLowerCase().includes(query.toLowerCase());
	}

	// Build the public URL for a mixed-type follow entry.
	function entityUrl(item: { entity_type: string; entity_slug?: string | null; entity_id: string }): string {
		const slug = item.entity_slug ? item.entity_slug : item.entity_id;
		switch (item.entity_type) {
			case 'user':
				return `/u/${slug}`;
			case 'organization':
				return `/organizations/${slug}`;
			case 'space':
				return `/spaces/${slug}`;
			case 'tag':
				return `/tags/${slug}`;
			case 'source':
				return `/sources/${slug}`;
			default:
				return `/${slug}`;
		}
	}

	const following = $derived(
		(rel?.following ?? [])
			.filter((f) => !optimisticRemoved[removedKey('following', f.entity_id)])
			.filter(
				(f) =>
					matchesSearch(f.entity_display_name || f.entity_name, tabSearch['following']) ||
					matchesSearch(f.entity_slug, tabSearch['following'])
			)
	);
	const followers = $derived(
		(rel?.followers ?? []).filter(
			(u) =>
				!optimisticRemoved[removedKey('followers', u.follower_id)] &&
				(matchesSearch(u.display_name || u.username, tabSearch['followers']) ||
					matchesSearch(u.username, tabSearch['followers']))
		)
	);
	const spaces = $derived(
		(rel?.spaces ?? []).filter((s) => !optimisticRemoved[removedKey('spaces', s.id)])
	);
	const tags = $derived(
		(rel?.tags ?? [])
			.filter((t) => !optimisticRemoved[removedKey('tags', t.entity_id)])
			.filter(
				(t) =>
					matchesSearch(t.entity_name, tabSearch['tags']) ||
					matchesSearch(t.entity_slug, tabSearch['tags'])
			)
	);
	const blocked = $derived(
		(rel?.blocked ?? [])
			.filter((u) => !optimisticRemoved[removedKey('blocked', u.blocked_id)])
			.filter(
				(u) =>
					matchesSearch(u.display_name || u.username, tabSearch['blocked']) ||
					matchesSearch(u.username, tabSearch['blocked'])
			)
	);
	const mutedUsers = $derived(
		(rel?.mutedUsers ?? [])
			.filter((u) => !optimisticRemoved[removedKey('muted-users', u.muted_user_id)])
			.filter(
				(u) =>
					matchesSearch(u.display_name || u.username, tabSearch['muted-users']) ||
					matchesSearch(u.username, tabSearch['muted-users'])
			)
	);
	const mutedSpaces = $derived(
		(rel?.mutedSpaces ?? []).filter((s) => !optimisticRemoved[removedKey('muted-spaces', s.space_id)])
	);

	function listForTab(tab: string): { length: number } {
		switch (tab) {
			case 'following':
				return following;
			case 'followers':
				return followers;
			case 'spaces':
				return spaces;
			case 'tags':
				return tags;
			case 'blocked':
				return blocked;
			case 'muted-users':
				return mutedUsers;
			case 'muted-spaces':
				return mutedSpaces;
			default:
				return { length: 0 };
		}
	}

	const countKeys: Record<string, keyof RelationshipCounts> = {
		following: 'following',
		followers: 'followers',
		spaces: 'spaces',
		tags: 'tags',
		blocked: 'blocked',
		'muted-users': 'mutedUsers',
		'muted-spaces': 'mutedSpaces'
	};

	function metricValue(tab: string): number | string {
		const key = countKeys[tab];
		const exact = key ? rel?.counts?.[key] : undefined;
		if (typeof exact === 'number') return exact;
		const list = listForTab(tab);
		const more = rel?.hasMore?.[tab] ?? false;
		return more ? `${list.length}+` : list.length;
	}

	function followableType(raw: string): FollowableType {
		return raw as FollowableType;
	}

	async function doUnfollow(item: FollowedEntity) {
		const type = followableType(item.entity_type);
		if ((type === 'space' || type === 'tag' || type === 'source') && !item.entity_slug) return;
		const key = removedKey('following', item.entity_id);
		const slug = item.entity_slug ? item.entity_slug : undefined;
		const { ok } = await optimisticAction({
			apply: () => optimisticRemoved[key] = true,
			rollback: () => delete optimisticRemoved[key],
			call: () => toggleFollow(type, item.entity_id, slug, true)
		});
		if (ok) {
			await invalidate('/api/v1/me/following');
		}
		delete optimisticRemoved[key];
	}

	async function doUnfollowTag(tag: FollowedEntity) {
		const slug = tag.entity_slug;
		if (!slug) return;
		const key = removedKey('tags', tag.entity_id);
		const { ok } = await optimisticAction({
			apply: () => optimisticRemoved[key] = true,
			rollback: () => delete optimisticRemoved[key],
			call: () => unfollowTag(slug)
		});
		if (ok) {
			await invalidate('/api/v1/me/followed-tags');
		}
		delete optimisticRemoved[key];
	}

	async function doLeaveSpace(space: Space) {
		const slug = space.slug;
		if (!slug) return;
		const key = removedKey('spaces', space.id);
		const { ok } = await optimisticAction({
			apply: () => optimisticRemoved[key] = true,
			rollback: () => delete optimisticRemoved[key],
			call: () => leaveSpace(slug)
		});
		if (ok) {
			await invalidate('/api/v1/me/spaces');
		}
		delete optimisticRemoved[key];
	}

	async function doUnblockUser(user: BlockedUser) {
		const key = removedKey('blocked', user.blocked_id);
		const { ok } = await optimisticAction({
			apply: () => optimisticRemoved[key] = true,
			rollback: () => delete optimisticRemoved[key],
			call: () => unblockUser(user.blocked_id)
		});
		if (ok) {
			await invalidate('/api/v1/me/blocked-users');
		}
		delete optimisticRemoved[key];
	}

	async function doUnmuteUser(user: MutedUser) {
		const key = removedKey('muted-users', user.muted_user_id);
		const { ok } = await optimisticAction({
			apply: () => optimisticRemoved[key] = true,
			rollback: () => delete optimisticRemoved[key],
			call: () => unmuteUser(user.muted_user_id)
		});
		if (ok) {
			await invalidate('/api/v1/me/muted-users');
		}
		delete optimisticRemoved[key];
	}

	async function doUnmuteSpace(space: MutedSpace) {
		const slug = space.space_slug;
		if (!slug) return;
		const key = removedKey('muted-spaces', space.space_id);
		const { ok } = await optimisticAction({
			apply: () => optimisticRemoved[key] = true,
			rollback: () => delete optimisticRemoved[key],
			call: () => unmuteSpace(slug)
		});
		if (ok) {
			await invalidate('/api/v1/me/muted-spaces');
		}
		delete optimisticRemoved[key];
	}
</script>

<section class="space-y-4">
	<header>
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('dashboard.nav.relationships')}</h1>
	</header>

	<div class="grid grid-cols-2 gap-4 sm:grid-cols-4">
		<MetricCard labelKey="dashboard.relationships.following_count" value={metricValue('following')} />
		<MetricCard labelKey="dashboard.relationships.followers_count" value={metricValue('followers')} />
		<MetricCard labelKey="dashboard.relationships.spaces_count" value={metricValue('spaces')} />
		<MetricCard labelKey="dashboard.relationships.tags_count" value={metricValue('tags')} />
	</div>

	<div class="flex flex-col gap-3">
		<div class="flex flex-wrap gap-2">
			{#each tabs as tab (tab.key)}
				<button
					type="button"
					class="rounded-[var(--radius)] px-3 py-1.5 text-sm {activeTab === tab.key
						? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)]'
						: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
					onclick={() => switchTab(tab.key)}
				>
					{t(tab.labelKey)}
				</button>
			{/each}
		</div>

		<div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
			<div class="relative w-full max-w-xs">
				<Search size={14} class="absolute left-2 top-1/2 -translate-y-1/2 text-[var(--color-muted)]" />
				<input
					type="search"
					value={searchInputValue}
					placeholder={t('dashboard.relationships.search', 'Search...')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] pl-7 pr-2 py-1.5 text-sm text-[var(--color-text)]"
					oninput={(e) => onSearchInput((e.currentTarget as HTMLInputElement).value)}
				/>
			</div>

			<div class="flex items-center gap-2">
				{#if activeTab === 'following'}
					<AutocompleteSelect
						class="w-48"
						value={typeFilter}
						options={followTypeOptions}
						onValueChange={(v) => setSearchParams({ type: v })}
					/>
				{/if}

				{#if activeTab === 'spaces' || activeTab === 'muted-spaces'}
					<AutocompleteSelect
						class="w-48"
						value={spaceTypeFilter}
						options={[
							{ value: '', label: t('dashboard.relationships.all_space_types', 'All types') },
							...spaceTypes.map((st) => ({ value: st.id, label: st.name }))
						]}
						onValueChange={(v) => setSearchParams({ space_type: v })}
					/>
					<AutocompleteSelect
						class="w-40"
						value={levelFilter}
						options={spaceLevelOptions}
						onValueChange={(v) => setSearchParams({ level: v })}
					/>
				{/if}

				<AutocompleteSelect
					class="w-24"
					value={String(pageSize)}
					options={pageSizeOptions}
					ariaLabel={t('dashboard.relationships.page_size', 'Rows per page')}
					onValueChange={(v) => setSearchParams({ page_size: v })}
				/>
			</div>
		</div>
	</div>

	{#if activeTab === 'following'}
		{#if following.length === 0}
			<EmptyState icon={Users} title={t('dashboard.relationships.following_empty')} />
		{:else}
			<ul class="grid grid-cols-1 gap-2 lg:grid-cols-2">
				{#each following as item (item.entity_id)}
					<li
						class="flex items-center justify-between gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3"
					>
						<div class="flex min-w-0 items-center gap-3">
							{#if item.entity_avatar_url}
								<img
									src={item.entity_avatar_url}
									alt={displayName(item) || item.entity_name || item.entity_id}
									class="h-10 w-10 shrink-0 rounded-full object-cover"
								/>
							{:else}
								<span
									class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/10 text-sm font-medium text-[var(--color-primary)]"
								>
									{initials(displayName(item) || item.entity_name)}
								</span>
							{/if}
							<div class="min-w-0">
								<a
									href={entityUrl(item)}
									class="block truncate text-sm font-medium text-[var(--color-text)] hover:underline"
								>
									{displayName(item) || item.entity_name || item.entity_id}
								</a>
								{#if item.created_at}
									<span class="text-xs text-[var(--color-muted)]">
										{t('dashboard.relationships.since', 'Since')} {formatDate(item.created_at)}
									</span>
								{/if}
							</div>
						</div>
						<button
							type="button"
							class="text-xs text-[var(--color-muted)] hover:text-[var(--color-danger)]"
							onclick={() => doUnfollow(item)}
						>
							{t('dashboard.relationships.unfollow')}
						</button>
					</li>
				{/each}
			</ul>
		{/if}
	{:else if activeTab === 'followers'}
		{#if followers.length === 0}
			<EmptyState icon={Users} title={t('dashboard.relationships.followers_empty')} />
		{:else}
			<ul class="grid grid-cols-1 gap-2 lg:grid-cols-2">
				{#each followers as user (user.follower_id)}
					<li
						class="flex items-center justify-between gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3"
					>
						<div class="flex min-w-0 items-center gap-3">
							{#if user.avatar_url}
								<img
									src={user.avatar_url}
									alt={user.display_name || user.username || user.follower_id}
									class="h-10 w-10 shrink-0 rounded-full object-cover"
								/>
							{:else}
								<span
									class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/10 text-sm font-medium text-[var(--color-primary)]"
								>
									{initials(user.display_name || user.username)}
								</span>
							{/if}
							<div class="min-w-0">
								<a
									href="/u/{user.username || user.follower_id}"
									class="block truncate text-sm font-medium text-[var(--color-text)] hover:underline"
								>
									{user.display_name || user.username || user.follower_id}
								</a>
								{#if user.created_at}
									<span class="text-xs text-[var(--color-muted)]">
										{t('dashboard.relationships.since', 'Since')} {formatDate(user.created_at)}
									</span>
								{/if}
							</div>
						</div>
						{#if user.is_following}
							<span class="text-xs text-[var(--color-muted)]">
								{t('dashboard.relationships.following_you', 'Following you')}
							</span>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	{:else if activeTab === 'spaces'}
		{#if spaces.length === 0}
			<EmptyState icon={Users} title={t('dashboard.relationships.spaces_empty')} />
		{:else}
			<ul class="grid grid-cols-1 gap-2 lg:grid-cols-2">
				{#each spaces as space (space.id)}
					<li
						class="flex items-center justify-between gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3"
					>
						<div class="flex min-w-0 items-center gap-3">
							<span
								class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/10 text-sm font-medium text-[var(--color-primary)]"
							>
								{initials(space.name)}
							</span>
							<div class="min-w-0">
								<a
									href="/spaces/{space.slug ?? space.id}"
									class="block truncate text-sm font-medium text-[var(--color-text)] hover:underline"
								>
									{space.name ?? space.slug ?? space.id}
								</a>
								{#if space.is_primary}
									<span
										class="inline-block rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-xs text-[var(--color-primary)]"
									>
										{t('dashboard.relationships.primary_space', 'Primary')}
									</span>
								{/if}
							</div>
						</div>
						<button
							type="button"
							class="text-xs text-[var(--color-muted)] hover:text-[var(--color-danger)]"
							onclick={() => doLeaveSpace(space)}
						>
							{t('dashboard.relationships.leave')}
						</button>
					</li>
				{/each}
			</ul>
		{/if}
	{:else if activeTab === 'tags'}
		{#if tags.length === 0}
			<EmptyState icon={Users} title={t('dashboard.relationships.tags_empty')} />
		{:else}
			<ul class="grid grid-cols-1 gap-2 lg:grid-cols-2">
				{#each tags as tag (tag.entity_id)}
					<li
						class="flex items-center justify-between gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3"
					>
						<a
							href="/tags/{tag.entity_slug || tag.entity_id}"
							class="text-sm font-medium text-[var(--color-text)] hover:underline"
						>
							#{tag.entity_name || tag.entity_id}
						</a>
						<button
							type="button"
							class="text-xs text-[var(--color-muted)] hover:text-[var(--color-danger)]"
							onclick={() => doUnfollowTag(tag)}
						>
							{t('dashboard.relationships.unfollow')}
						</button>
					</li>
				{/each}
			</ul>
		{/if}
	{:else if activeTab === 'blocked'}
		{#if blocked.length === 0}
			<EmptyState icon={Users} title={t('dashboard.relationships.blocked_empty')} />
		{:else}
			<ul class="grid grid-cols-1 gap-2 lg:grid-cols-2">
				{#each blocked as user (user.blocked_id)}
					<li
						class="flex items-center justify-between gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3"
					>
						<div class="flex min-w-0 items-center gap-3">
							{#if user.avatar_url}
								<img
									src={user.avatar_url}
									alt={user.display_name || user.username || user.blocked_id}
									class="h-10 w-10 shrink-0 rounded-full object-cover"
								/>
							{:else}
								<span
									class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/10 text-sm font-medium text-[var(--color-primary)]"
								>
									{initials(user.display_name || user.username)}
								</span>
							{/if}
							<a
								href="/u/{user.username ?? user.blocked_id}"
								class="block truncate text-sm font-medium text-[var(--color-text)] hover:underline"
							>
								{user.display_name ?? user.username ?? user.blocked_id}
							</a>
						</div>
						<button
							type="button"
							class="text-xs text-[var(--color-muted)] hover:text-[var(--color-danger)]"
							onclick={() => doUnblockUser(user)}
						>
							{t('dashboard.relationships.unblock')}
						</button>
					</li>
				{/each}
			</ul>
		{/if}
	{:else if activeTab === 'muted-users'}
		{#if mutedUsers.length === 0}
			<EmptyState icon={Users} title={t('dashboard.relationships.muted_users_empty', "You haven't muted anyone")} />
		{:else}
			<ul class="grid grid-cols-1 gap-2 lg:grid-cols-2">
				{#each mutedUsers as user (user.muted_user_id)}
					<li
						class="flex items-center justify-between gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3"
					>
						<div class="flex min-w-0 items-center gap-3">
							{#if user.avatar_url}
								<img
									src={user.avatar_url}
									alt={user.display_name || user.username || user.muted_user_id}
									class="h-10 w-10 shrink-0 rounded-full object-cover"
								/>
							{:else}
								<span
									class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/10 text-sm font-medium text-[var(--color-primary)]"
								>
									{initials(user.display_name || user.username)}
								</span>
							{/if}
							<a
								href="/u/{user.username ?? user.muted_user_id}"
								class="block truncate text-sm font-medium text-[var(--color-text)] hover:underline"
							>
								{user.display_name ?? user.username ?? user.muted_user_id}
							</a>
						</div>
						<button
							type="button"
							class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
							onclick={() => doUnmuteUser(user)}
						>
							{t('dashboard.relationships.unmute', 'Unmute')}
						</button>
					</li>
				{/each}
			</ul>
		{/if}
	{:else if activeTab === 'muted-spaces'}
		{#if mutedSpaces.length === 0}
			<EmptyState icon={Users} title={t('dashboard.relationships.muted_spaces_empty', "You haven't muted any spaces")} />
		{:else}
			<ul class="grid grid-cols-1 gap-2 lg:grid-cols-2">
				{#each mutedSpaces as space (space.space_id)}
					<li
						class="flex items-center justify-between gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3"
					>
						<div class="flex min-w-0 items-center gap-3">
							<span
								class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/10 text-sm font-medium text-[var(--color-primary)]"
							>
								{initials(space.space_name)}
							</span>
							<div class="min-w-0">
								<a
									href="/spaces/{space.space_slug ?? space.space_id}"
									class="block truncate text-sm font-medium text-[var(--color-text)] hover:underline"
								>
									{space.space_name ?? space.space_slug ?? space.space_id}
								</a>
								{#if space.is_primary}
									<span
										class="inline-block rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-xs text-[var(--color-primary)]"
									>
										{t('dashboard.relationships.primary_space', 'Primary')}
									</span>
								{/if}
							</div>
						</div>
						<button
							type="button"
							class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
							onclick={() => doUnmuteSpace(space)}
						>
							{t('dashboard.relationships.unmute', 'Unmute')}
						</button>
					</li>
				{/each}
			</ul>
		{/if}
	{/if}

	{#if activeTab !== 'following' || following.length > 0}
		<div class="flex items-center justify-between gap-2 pt-2">
			<button
				type="button"
				class="flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] disabled:opacity-40"
				disabled={currentPage <= 1}
				onclick={() => goPage(-1)}
			>
				<ChevronLeft size={16} />
				{t('dashboard.relationships.previous', 'Previous')}
			</button>
			<span class="text-sm text-[var(--color-muted)]">{t('dashboard.relationships.page', 'Page')} {currentPage}</span>
			<button
				type="button"
				class="flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] disabled:opacity-40"
				disabled={!rel?.hasMore?.[activeTab]}
				onclick={() => goPage(1)}
			>
				{t('dashboard.relationships.next', 'Next')}
				<ChevronRight size={16} />
			</button>
		</div>
	{/if}
</section>

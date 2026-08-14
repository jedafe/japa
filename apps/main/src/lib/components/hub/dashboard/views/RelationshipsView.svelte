<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { unfollowUser, leaveSpace, unfollowTag, unblockUser, unmuteUser, unmuteSpace } from '$lib/api/hub';
	import { optimisticAction } from '$lib/components/hub/optimisticAction';
	import { invalidate } from '$app/navigation';
	import MetricCard from '$lib/components/hub/MetricCard.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { Users } from 'lucide-svelte';

	// The backend returns a uniform FollowingInfo shape for /me/following and
	// /me/followed-tags: { id, entity_type, entity_id, entity_name, entity_slug }.
	// The dashboard maps these to type-specific display fields.
	interface FollowedEntity {
		id: string;
		entity_type: string;
		entity_id: string;
		entity_name: string;
		entity_slug: string;
	}
	interface Space { id: string; slug?: string; name?: string; }
	interface BlockedUser { blocked_id: string; username?: string; display_name?: string; created_at?: string; }
	interface MutedUser { muted_user_id: string; username?: string; display_name?: string; created_at?: string; }
	interface MutedSpace { space_id: string; space_name?: string; space_slug?: string; created_at?: string; }

	interface RelationshipsData {
		following: FollowedEntity[];
		spaces: Space[];
		tags: FollowedEntity[];
		blocked?: BlockedUser[];
		mutedUsers?: MutedUser[];
		mutedSpaces?: MutedSpace[];
	}

	let { data }: { data: unknown } = $props();
	const rel = $derived(data as RelationshipsData | null);
	let following = $state<FollowedEntity[]>(untrack(() => rel?.following ?? []));
	let spaces = $state<Space[]>(untrack(() => rel?.spaces ?? []));
	let tags = $state<FollowedEntity[]>(untrack(() => rel?.tags ?? []));
	let blocked = $state<BlockedUser[]>(untrack(() => rel?.blocked ?? []));
	let mutedUsers = $state<MutedUser[]>(untrack(() => rel?.mutedUsers ?? []));
	let mutedSpaces = $state<MutedSpace[]>(untrack(() => rel?.mutedSpaces ?? []));

	const activeTab = $derived(page.url.searchParams.get('tab') ?? 'following');

	const tabs: { key: string; labelKey: string }[] = [
		{ key: 'following', labelKey: 'dashboard.relationships.tab_following' },
		{ key: 'spaces', labelKey: 'dashboard.relationships.tab_spaces' },
		{ key: 'tags', labelKey: 'dashboard.relationships.tab_tags' },
		{ key: 'blocked', labelKey: 'dashboard.relationships.tab_blocked' },
		{ key: 'muted-users', labelKey: 'dashboard.relationships.tab_muted_users' },
		{ key: 'muted-spaces', labelKey: 'dashboard.relationships.tab_muted_spaces' }
	];

	function switchTab(key: string) {
		goto(`/dashboard/relationships?tab=${key}`, { keepFocus: true, noScroll: true });
	}

	async function doUnfollowUser(entityId: string) {
		await optimisticAction({
			apply: () => (following = following.filter((u) => u.entity_id !== entityId)),
			rollback: () => (following = rel?.following ?? []),
			call: () => unfollowUser(entityId)
		});
		await invalidate('/api/v1/me/following');
	}

	async function doLeaveSpace(slug: string) {
		await optimisticAction({
			apply: () => (spaces = spaces.filter((s) => s.slug !== slug)),
			rollback: () => (spaces = rel?.spaces ?? []),
			call: () => leaveSpace(slug)
		});
		await invalidate('/api/v1/me/spaces');
	}

	async function doUnfollowTag(slug: string) {
		await optimisticAction({
			apply: () => (tags = tags.filter((t) => t.entity_slug !== slug)),
			rollback: () => (tags = rel?.tags ?? []),
			call: () => unfollowTag(slug)
		});
		await invalidate('/api/v1/me/followed-tags');
	}

	async function doUnblockUser(id: string) {
		await optimisticAction({
			apply: () => (blocked = blocked.filter((u) => u.blocked_id !== id)),
			rollback: () => (blocked = rel?.blocked ?? []),
			call: () => unblockUser(id)
		});
		await invalidate('/api/v1/me/blocked-users');
	}

	async function doUnmuteUser(id: string) {
		await optimisticAction({
			apply: () => (mutedUsers = mutedUsers.filter((u) => u.muted_user_id !== id)),
			rollback: () => (mutedUsers = rel?.mutedUsers ?? []),
			call: () => unmuteUser(id)
		});
		await invalidate('/api/v1/me/muted-users');
	}

	async function doUnmuteSpace(slug: string) {
		await optimisticAction({
			apply: () => (mutedSpaces = mutedSpaces.filter((s) => s.space_slug !== slug)),
			rollback: () => (mutedSpaces = rel?.mutedSpaces ?? []),
			call: () => unmuteSpace(slug)
		});
		await invalidate('/api/v1/me/muted-spaces');
	}
</script>

<section class="space-y-4">
	<header>
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('dashboard.nav.relationships')}</h1>
	</header>

	<div class="grid grid-cols-3 gap-4">
		<MetricCard labelKey="dashboard.relationships.following_count" value={following.length} />
		<MetricCard labelKey="dashboard.relationships.spaces_count" value={spaces.length} />
		<MetricCard labelKey="dashboard.relationships.tags_count" value={tags.length} />
	</div>

	<div class="flex gap-2">
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

	{#if activeTab === 'following'}
		{#if following.length === 0}
			<EmptyState icon={Users} title={t('dashboard.relationships.following_empty')} />
		{:else}
			<ul class="space-y-2">
				{#each following as user (user.entity_id)}
					<li class="flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
						<a href="/u/{user.entity_slug || user.entity_id}" class="text-sm font-medium text-[var(--color-text)] hover:underline">
							{user.entity_name || user.entity_id}
						</a>
						<button type="button" class="text-xs text-[var(--color-muted)] hover:text-[var(--color-danger)]" onclick={() => doUnfollowUser(user.entity_id)}>
							{t('dashboard.relationships.unfollow')}
						</button>
					</li>
				{/each}
			</ul>
		{/if}
	{:else if activeTab === 'spaces'}
		{#if spaces.length === 0}
			<EmptyState icon={Users} title={t('dashboard.relationships.spaces_empty')} />
		{:else}
			<ul class="space-y-2">
				{#each spaces as space (space.id)}
					<li class="flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
						<a href="/spaces/{space.slug ?? space.id}" class="text-sm font-medium text-[var(--color-text)] hover:underline">
							{space.name ?? space.slug ?? space.id}
						</a>
						<button type="button" class="text-xs text-[var(--color-muted)] hover:text-[var(--color-danger)]" onclick={() => doLeaveSpace(space.slug ?? space.id)}>
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
			<ul class="space-y-2">
				{#each tags as tag (tag.entity_id)}
					<li class="flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
						<a href="/tags/{tag.entity_slug || tag.entity_id}" class="text-sm font-medium text-[var(--color-text)] hover:underline">
							#{tag.entity_name || tag.entity_id}
						</a>
						<button type="button" class="text-xs text-[var(--color-muted)] hover:text-[var(--color-danger)]" onclick={() => doUnfollowTag(tag.entity_slug || tag.entity_id)}>
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
			<ul class="space-y-2">
				{#each blocked as user (user.blocked_id)}
					<li class="flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
						<a href="/u/{user.username ?? user.blocked_id}" class="text-sm font-medium text-[var(--color-text)] hover:underline">
							{user.display_name ?? user.username ?? user.blocked_id}
						</a>
						<button type="button" class="text-xs text-[var(--color-muted)] hover:text-[var(--color-danger)]" onclick={() => doUnblockUser(user.blocked_id)}>
							{t('dashboard.relationships.unblock')}
						</button>
					</li>
				{/each}
			</ul>
		{/if}
	{:else if activeTab === 'muted-users'}
		{#if mutedUsers.length === 0}
			<EmptyState icon={Users} title={t('dashboard.relationships.muted_users_empty', 'You haven\'t muted anyone')} />
		{:else}
			<ul class="space-y-2">
				{#each mutedUsers as user (user.muted_user_id)}
					<li class="flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
						<a href="/u/{user.username ?? user.muted_user_id}" class="text-sm font-medium text-[var(--color-text)] hover:underline">
							{user.display_name ?? user.username ?? user.muted_user_id}
						</a>
						<button type="button" class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]" onclick={() => doUnmuteUser(user.muted_user_id)}>
							{t('dashboard.relationships.unmute', 'Unmute')}
						</button>
					</li>
				{/each}
			</ul>
		{/if}
	{:else if activeTab === 'muted-spaces'}
		{#if mutedSpaces.length === 0}
			<EmptyState icon={Users} title={t('dashboard.relationships.muted_spaces_empty', 'You haven\'t muted any spaces')} />
		{:else}
			<ul class="space-y-2">
				{#each mutedSpaces as space (space.space_id)}
					<li class="flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
						<a href="/spaces/{space.space_slug ?? space.space_id}" class="text-sm font-medium text-[var(--color-text)] hover:underline">
							{space.space_name ?? space.space_slug ?? space.space_id}
						</a>
						<button type="button" class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]" onclick={() => doUnmuteSpace(space.space_slug ?? space.space_id)}>
							{t('dashboard.relationships.unmute', 'Unmute')}
						</button>
					</li>
				{/each}
			</ul>
		{/if}
	{/if}
</section>

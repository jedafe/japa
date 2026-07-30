<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { unfollowUser, leaveSpace, unfollowTag } from '$lib/api/hub';
	import { optimisticAction } from '$lib/components/hub/optimisticAction';
	import { invalidate } from '$app/navigation';
	import MetricCard from '$lib/components/hub/MetricCard.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { Users } from 'lucide-svelte';

	interface FollowedUser { id: string; username?: string; display_name?: string; }
	interface Space { id: string; slug?: string; name?: string; }
	interface FollowedTag { id: string; slug?: string; name?: string; }

	interface RelationshipsData {
		following: FollowedUser[];
		spaces: Space[];
		tags: FollowedTag[];
	}

	let { data }: { data: unknown } = $props();
	const rel = $derived(data as RelationshipsData | null);
	let following = $state<FollowedUser[]>(untrack(() => rel?.following ?? []));
	let spaces = $state<Space[]>(untrack(() => rel?.spaces ?? []));
	let tags = $state<FollowedTag[]>(untrack(() => rel?.tags ?? []));

	const activeTab = $derived(page.url.searchParams.get('tab') ?? 'following');

	const tabs: { key: string; labelKey: string }[] = [
		{ key: 'following', labelKey: 'dashboard.relationships.tab_following' },
		{ key: 'spaces', labelKey: 'dashboard.relationships.tab_spaces' },
		{ key: 'tags', labelKey: 'dashboard.relationships.tab_tags' }
	];

	function switchTab(key: string) {
		goto(`/dashboard/relationships?tab=${key}`, { keepFocus: true, noScroll: true });
	}

	async function doUnfollowUser(id: string) {
		await optimisticAction({
			apply: () => (following = following.filter((u) => u.id !== id)),
			rollback: () => (following = rel?.following ?? []),
			call: () => unfollowUser(id)
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
			apply: () => (tags = tags.filter((t) => t.slug !== slug)),
			rollback: () => (tags = rel?.tags ?? []),
			call: () => unfollowTag(slug)
		});
		await invalidate('/api/v1/me/followed-tags');
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
				{#each following as user (user.id)}
					<li class="flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
						<a href="/u/{user.username ?? user.id}" class="text-sm font-medium text-[var(--color-text)] hover:underline">
							{user.display_name ?? user.username ?? user.id}
						</a>
						<button type="button" class="text-xs text-[var(--color-muted)] hover:text-[var(--color-danger)]" onclick={() => doUnfollowUser(user.id)}>
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
				{#each tags as tag (tag.id)}
					<li class="flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
						<a href="/tags/{tag.slug ?? tag.id}" class="text-sm font-medium text-[var(--color-text)] hover:underline">
							#{tag.name ?? tag.slug ?? tag.id}
						</a>
						<button type="button" class="text-xs text-[var(--color-muted)] hover:text-[var(--color-danger)]" onclick={() => doUnfollowTag(tag.slug ?? tag.id)}>
							{t('dashboard.relationships.unfollow')}
						</button>
					</li>
				{/each}
			</ul>
		{/if}
	{/if}
</section>

<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { Compass, Users, Building2, Hash, GraduationCap, FileText, Globe, BadgeCheck } from 'lucide-svelte';
	import { EmptyState, UrlTabs, FollowButton, SpaceJoinButton, DirectoryCard, type UrlTab, type CardMetaItem } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import type { PageData } from './$types';
	import type { PublicOrgListItem, University } from '$lib/types';

	let { data }: { data: PageData } = $props();

	const tab = $derived(data.tab);

	// Tab bar values — switching calls goto() so SvelteKit re-runs the load
	// function for the new tab's per-tab data.
	const tabs = $derived<UrlTab[]>([
		{ value: 'spaces', label: t('discover.tab.spaces', 'spaces') },
		{ value: 'people', label: t('discover.tab.people', 'people') },
		{ value: 'organizations', label: t('discover.tab.organizations', 'organizations') },
		{ value: 'tags', label: t('discover.tab.tags', 'tags') },
		{ value: 'content', label: t('discover.tab.content', 'Content') },
		{ value: 'universities', label: t('discover.tab.universities', 'universities') },
		{ value: 'sources', label: t('discover.tab.sources', 'Sources') }
	]);

	const title = $derived(t('discover.title', 'Discover — JapaConnect'));
	const description = $derived(
		t('discover.description', 'Find spaces, people, organizations, tags, sources, and universities in the Nigerian diaspora community.')
	);

	// ── Load-more state for the server-paginated tabs (organizations,
	// universities). The suggestions tabs are fixed pools with a limit-only
	// contract — there is no second page to fetch.
	let extraOrgs = $state<PublicOrgListItem[]>([]);
	let extraUnis = $state<University[]>([]);
	let unisPage = $state(1);
	let loadingMore = $state(false);

	const organizations = $derived([...data.organizations, ...extraOrgs]);
	const universities = $derived([...data.universities, ...extraUnis]);
	const orgsHasMore = $derived(organizations.length < data.orgsTotal);
	const unisHasMore = $derived(universities.length < data.unisTotal);

	// Follow state comes from /me/following (loaded server-side) — the follow
	// endpoints are toggles, so an accurate initiallyFollowing is required or a
	// "Follow" click on an already-followed entity would unfollow it.
	const followedSpaceIds = $derived(new Set(data.followedSpaceIds));
	const followedOrgIds = $derived(new Set(data.followedOrgIds));
	const followedSourceIds = $derived(new Set(data.followedSourceIds));

	const count = (key: string, fallback: string, n: number) =>
		t(key, fallback).replace('{n}', String(n));

	const meta = (...items: (CardMetaItem | null)[]): CardMetaItem[] =>
		items.filter((i): i is CardMetaItem => i !== null);

	// University avatar text — seeded initials can be long codes (e.g.
	// "VERMONTLAW"); anything over 5 chars overflows the badge, so fall back
	// to the first letter of the name.
	const uniMark = (uni: { name: string; initials?: string | null }) =>
		uni.initials && uni.initials.length <= 5 ? uni.initials : uni.name.charAt(0).toUpperCase();

	// Reset client-appended pages whenever the active tab (and its SSR
	// payload) changes.
	$effect(() => {
		void data.tab;
		extraOrgs = [];
		extraUnis = [];
		unisPage = 1;
	});

	async function loadMoreOrgs() {
		if (loadingMore) return;
		loadingMore = true;
		// GET /organizations accepts page_size + page_offset (raw row offset).
		const res = await api.get<PublicOrgListItem[]>(
			`/organizations?page_size=${data.pageSize}&page_offset=${organizations.length}`
		);
		loadingMore = false;
		if (res.data?.length) extraOrgs = [...extraOrgs, ...res.data];
	}

	async function loadMoreUnis() {
		if (loadingMore) return;
		loadingMore = true;
		// GET /universities paginates with ?page=&limit= (1-based page number).
		const res = await api.get<University[]>(
			`/universities?page=${unisPage + 1}&limit=${data.pageSize}`
		);
		loadingMore = false;
		if (res.data?.length) {
			extraUnis = [...extraUnis, ...res.data];
			unisPage += 1;
		}
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

<div class="mx-auto w-full max-w-full" style="--page-gutter: 1rem">
	<div class="border-b border-[var(--color-border)] px-4 py-3" style="margin-inline: max(1rem, calc((100% - 48rem)/2))">
		<h1 class="text-lg font-semibold text-[var(--color-text)]">{t('discover.heading', 'Discover')}</h1>
	</div>

	<!-- Tab bar -->
	<nav
		class="discover-tabs sticky top-0 z-10" style="margin-inline: max(0.75rem, calc((100% - 48rem)/2))"
		aria-label={t('discover.tabs_label', 'Discover tabs')}
	>
		<UrlTabs {tabs} value={data.tab} onValueChange={(v) => goto(`?tab=${v}`)} />
	</nav>

	<div class="mx-auto w-full max-w-3xl p-4">
		{#if tab === 'spaces'}
			{#if data.spaces.length === 0}
				<EmptyState icon={Compass} title={t('discover.spaces_empty', 'No space suggestions yet')} />
			{:else}
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
					{#each data.spaces as space (space.id)}
						<DirectoryCard
							href="/spaces/{space.slug}"
							title={space.name}
							subtitle={count('meta.members', '{n} members', space.member_count)}
							fallback={space.name.charAt(0).toUpperCase()}
							hueSeed={space.id}
							meta={meta(
								{ icon: FileText, text: count('discover.posts', '{n} posts', space.post_count) },
								space.shared_members > 0 ? { icon: Users, text: count('discover.mutual_members', '{n} mutual members', space.shared_members) } : null
							)}
							viewProfile
							viewProfileLabel={t('discover.view_space', 'View space')}
						>
							{#snippet headerAction()}
								<SpaceJoinButton entityId={space.id} entitySlug={space.slug} initiallyFollowing={followedSpaceIds.has(space.id)} compact />
							{/snippet}
						</DirectoryCard>
					{/each}
				</div>
			{/if}
		{:else if tab === 'people'}
			{#if data.people.length === 0}
				<EmptyState icon={Users} title={t('discover.people_empty', 'No people suggestions yet')} />
			{:else}
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
					{#each data.people as person (person.id)}
						<DirectoryCard
							href="/u/{person.username}"
							title={person.display_name || person.username}
							subtitle="@{person.username}"
							description={person.bio}
							imageUrl={person.avatar_url}
							fallback={(person.display_name || person.username).charAt(0).toUpperCase()}
							hueSeed={person.id}
							round
							meta={meta(
								person.shared_spaces > 0 ? { icon: Compass, text: count('discover.shared_spaces', '{n} spaces in common', person.shared_spaces) } : null
							)}
							viewProfile
							viewProfileLabel={t('discover.view_profile', 'View profile')}
						>
							{#snippet headerAction()}
								<FollowButton entityType="user" entityId={person.id} compact />
							{/snippet}
						</DirectoryCard>
					{/each}
				</div>
			{/if}
		{:else if tab === 'organizations'}
			{#if organizations.length === 0}
				<EmptyState icon={Building2} title={t('discover.orgs_empty', 'No organizations yet')} />
			{:else}
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
					{#each organizations as org (org.id)}
						<DirectoryCard
							href="/org/{org.slug}"
							title={org.name}
							subtitle="{org.org_type.replace(/_/g, ' ')}{org.country_code ? ` · ${org.country_code}` : ''}"
							description={org.tagline}
							fallback={org.name.charAt(0).toUpperCase()}
							hueSeed={org.id}
							verified={org.verified}
							meta={meta(
								{ icon: Users, text: count('profile.followers', '{n} followers', org.follower_count) },
								{ icon: Building2, text: count('meta.members', '{n} members', org.member_count) }
							)}
							viewProfile
							viewProfileLabel={t('discover.view_organization', 'View organization')}
						>
							{#snippet headerAction()}
								<FollowButton entityType="organization" entityId={org.id} initiallyFollowing={followedOrgIds.has(org.id)} compact />
							{/snippet}
						</DirectoryCard>
					{/each}
				</div>
				{#if orgsHasMore}
					<button
						type="button"
						onclick={loadMoreOrgs}
						disabled={loadingMore}
						class="mt-3 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-60"
					>
						{t('discover.load_more', 'Load more')}
					</button>
				{/if}
			{/if}
		{:else if tab === 'tags'}
			{#if data.tags.length === 0}
				<EmptyState icon={Hash} title={t('discover.tags_empty', 'No tag suggestions yet')} />
			{:else}
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
					{#each data.tags as tag (tag.id)}
						<DirectoryCard
							href="/tags/{tag.slug}"
							title="#{tag.name}"
							subtitle={count('profile.followers', '{n} followers', tag.follower_count)}
							icon={Hash}
							hueSeed={tag.id}
							round
							viewProfile
							viewProfileLabel={t('discover.explore_tag', 'Explore')}
						>
							{#snippet headerAction()}
								<FollowButton entityType="tag" entityId={tag.id} entitySlug={tag.slug} compact />
							{/snippet}
						</DirectoryCard>
					{/each}
				</div>
			{/if}
		{:else if tab === 'content'}
			{#if data.content.length === 0}
				<EmptyState icon={FileText} title={t('discover.content_empty', 'No content suggestions yet')} />
			{:else}
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
					{#each data.content as item (item.id)}
						<DirectoryCard
							href="/content/{item.slug}"
							title={item.title}
							subtitle={item.content_type}
							icon={FileText}
							hueSeed={item.id}
							viewProfile
							viewProfileLabel={t('discover.read', 'Read')}
						/>
					{/each}
				</div>
			{/if}
		{:else if tab === 'universities'}
			{#if universities.length === 0}
				<EmptyState icon={GraduationCap} title={t('discover.universities_empty', 'No universities yet')} />
			{:else}
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
					{#each universities as uni (uni.id)}
						<DirectoryCard
							href="/universities/{uni.slug}"
							title={uni.name}
							subtitle="{uni.geo_code ? `${uni.geo_code} · ` : ''}{count('meta.members', '{n} members', uni.member_count)}"
							description={uni.description}
							imageUrl={uni.logo_url}
							fallback={uniMark(uni)}
							hueSeed={uni.id}
							meta={meta(
								{ icon: FileText, text: count('discover.posts', '{n} posts', uni.post_count) },
								{ icon: Users, text: count('profile.followers', '{n} followers', uni.follower_count) }
							)}
							viewProfile
							viewProfileLabel={t('discover.view_university', 'View university')}
						>
							{#snippet headerAction()}
								<SpaceJoinButton entityId={uni.id} entitySlug={uni.slug} initiallyJoined={uni.is_member ?? false} initiallyPending={uni.is_pending ?? false} initiallyFollowing={uni.is_following ?? false} compact />
							{/snippet}
						</DirectoryCard>
					{/each}
				</div>
				{#if unisHasMore}
					<button
						type="button"
						onclick={loadMoreUnis}
						disabled={loadingMore}
						class="mt-3 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-60"
					>
						{t('discover.load_more', 'Load more')}
					</button>
				{/if}
			{/if}
		{:else if tab === 'sources'}
			{#if data.sources.length === 0}
				<EmptyState icon={Globe} title={t('discover.sources_empty', 'No sources yet')} />
			{:else}
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
					{#each data.sources as source (source.id)}
						<DirectoryCard
							href="/sources/{source.slug}"
							title={source.name}
							subtitle={source.domain}
							description={source.description}
							imageUrl={source.logo_url}
							fallback={source.name.charAt(0).toUpperCase()}
							hueSeed={source.id}
							meta={meta(
								{ icon: FileText, text: count('discover.posts', '{n} posts', source.post_count) },
								{ icon: Users, text: count('profile.followers', '{n} followers', source.follower_count) },
								source.trust_level > 0 ? { icon: BadgeCheck, text: `${t('sources.trust_level', 'Trust')} ${source.trust_level}` } : null
							)}
							viewProfile
							viewProfileLabel={t('discover.view_source', 'View source')}
						>
							{#snippet headerAction()}
								<FollowButton entityType="source" entityId={source.id} entitySlug={source.slug} initiallyFollowing={source.is_following ?? followedSourceIds.has(source.id)} compact />
							{/snippet}
						</DirectoryCard>
					{/each}
				</div>
			{/if}
		{/if}
	</div>
</div>

<style>
	.discover-tabs {
		scrollbar-width: none;
	}
	.discover-tabs::-webkit-scrollbar {
		display: none;
	}
</style>

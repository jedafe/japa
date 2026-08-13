<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto, invalidate } from '$app/navigation';
	import { t } from '$lib/i18n';
	import { Radar, Pin, Loader2, Lock, Globe, ChevronRight } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import { requireAuthOrRedirect } from '$lib/auth/guest';
	import RadarItemRow, { type RadarItem } from '$lib/components/RadarItemRow.svelte';
	import type { Space } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('radar.title', 'Japa Radar — JapaConnect'));
	const description = $derived(
		t('radar.description', 'Track scholarships and opportunities across the countries you follow.')
	);

	const authed = $derived(data.authed);
	const intelUnavailable = $derived(data.intelUnavailable);

	// Followed feed items + country browse list (SSR-seeded).
	const followedItems = $derived<RadarItem[]>(data.followedItems);
	const countries = $derived<Space[]>(data.countries);

	// Followed space IDs — seeded from SSR, mutated optimistically on toggle.
	let followedSpaceIds = $state<Set<string>>(untrack(() => new Set(data.followedSpaceIds ?? [])));

	// Group the followed feed by country so each followed country renders as
	// a pinned section with its items beneath.
	const followedByCountry = $derived.by(() => {
		const map = new Map<string, RadarItem[]>();
		for (const item of followedItems) {
			const key = item.country || t('radar.uncategorized', 'Uncategorized');
			const arr = map.get(key);
			if (arr) arr.push(item);
			else map.set(key, [item]);
		}
		return Array.from(map.entries());
	});

	// Countries the user follows — pinned to the top of the browse list.
	const followedCountries = $derived(
		countries.filter((c) => followedSpaceIds.has(c.id))
	);
	const browseCountries = $derived(
		countries.filter((c) => !followedSpaceIds.has(c.id))
	);

	// "Load more" — appends the next page of followed items client-side.
	let loadingMore = $state(false);
	let extraItems = $state<RadarItem[]>([]);
	const allFollowedItems = $derived([...followedItems, ...extraItems]);
	const hasMore = $derived(followedItems.length >= data.pageSize);

	async function loadMore() {
		if (loadingMore || !authed) return;
		loadingMore = true;
		const nextOffset = allFollowedItems.length;
		const res = await api.get<RadarItem[]>(
			`/intel/radar/followed?page_size=${data.pageSize}&page_offset=${nextOffset}`
		);
		if (!res.error && res.data) {
			extraItems = [...extraItems, ...res.data];
		}
		loadingMore = false;
	}

	// Follow / unfollow a country space. Calls POST/DELETE on the radar
	// follow endpoint, then invalidates the page load so the SSR data
	// (followed feed + followed_space_ids) re-fetches from the server.
	let pendingSpaceId = $state<string | null>(null);

	async function toggleFollow(space: Space) {
		if (pendingSpaceId) return;
		if (!(await requireAuthOrRedirect())) return;
		const wasFollowing = followedSpaceIds.has(space.id);
		// optimistic
		const next = new Set(followedSpaceIds);
		if (wasFollowing) next.delete(space.id);
		else next.add(space.id);
		followedSpaceIds = next;
		pendingSpaceId = space.id;

		let ok = false;
		if (wasFollowing) {
			const res = await api.delete<{ following: boolean }>(
				`/intel/radar/follow?space_id=${encodeURIComponent(space.id)}`
			);
			ok = !res.error;
		} else {
			const res = await api.post<{ following: boolean }>(
				`/intel/radar/follow`,
				{ space_id: space.id }
			);
			ok = !res.error;
		}

		if (!ok) {
			// rollback
			const rollback = new Set(followedSpaceIds);
			if (wasFollowing) rollback.add(space.id);
			else rollback.delete(space.id);
			followedSpaceIds = rollback;
		} else {
			// Re-run the page load so the followed feed + badges stay in sync.
			await invalidate(`/api/v1/intel/radar/followed`);
		}
		pendingSpaceId = null;
	}

	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'CollectionPage',
			name: t('radar.title', 'Japa Radar'),
			description,
			url: page.url.href
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

<div class="mx-auto w-full max-w-4xl">
	<!-- Header -->
	<div class="border-b border-[var(--color-border)] px-4 py-4">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Radar size={20} /> {t('radar.heading', 'Japa Radar')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{description}</p>
	</div>

	{#if intelUnavailable}
		<div class="p-4">
			<EmptyState
				icon={Lock}
				title={t('radar.unavailable_title', 'Radar is currently unavailable')}
				body={t('radar.unavailable_body', 'The intelligence service is not configured. Check back soon.')}
			/>
		</div>
	{:else}
		<!-- Followed countries feed — pinned to top (authed only) -->
		{#if authed}
			<div class="border-b border-[var(--color-border)] px-4 py-4">
				<h2 class="mb-3 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
					<Pin size={16} class="text-[var(--color-primary)]" />
					{t('radar.followed', 'Following')}
					{#if data.followedTotal > 0}
						<span class="text-xs font-normal text-[var(--color-muted)]">
							({data.followedTotal} {t('radar.countries', 'countries')})
						</span>
					{/if}
				</h2>

				{#if allFollowedItems.length === 0}
					<EmptyState
						icon={Radar}
						title={t('radar.empty_followed', 'No opportunities in your followed countries yet')}
						body={t('radar.empty_followed_body', 'Follow countries below to surface their latest scholarships here.')}
					/>
				{:else}
					<div class="space-y-5">
						{#each followedByCountry as [country, items] (country)}
							<section>
								<h3 class="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
									<Globe size={12} /> {country}
									<span class="font-normal normal-case text-[var(--color-muted)]">· {items.length}</span>
								</h3>
								<div class="space-y-2">
									{#each items as item (item.id)}
										<RadarItemRow {item} />
									{/each}
								</div>
							</section>
						{/each}
					</div>

					{#if hasMore}
						<div class="mt-4 flex justify-center">
							<button
								type="button"
								onclick={loadMore}
								disabled={loadingMore}
								aria-busy={loadingMore}
								class="inline-flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
							>
								{#if loadingMore}
									<Loader2 size={16} class="animate-spin" />
								{/if}
								{t('radar.load_more', 'Load more')}
							</button>
						</div>
					{/if}
				{/if}
			</div>
		{:else}
			<div class="border-b border-[var(--color-border)] px-4 py-3">
				<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-xs text-[var(--color-text-subtle)]">
					{t('radar.signin_prompt', 'Sign in to follow countries and get a personalized radar feed.')}
				</div>
			</div>
		{/if}

		<!-- Browse countries — followed pinned to top, then the rest -->
		<div class="p-4">
			<h2 class="mb-3 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
				<Globe size={16} />
				{t('radar.browse_countries', 'Browse countries')}
				{#if countries.length > 0}
					<span class="text-xs font-normal text-[var(--color-muted)]">({countries.length})</span>
				{/if}
			</h2>

			{#if data.countriesError}
				<p class="py-8 text-center text-sm text-[var(--color-text-subtle)]">
					{t('radar.countries_error', 'Failed to load the country list. Please try again later.')}
				</p>
			{:else if countries.length === 0}
				<EmptyState
					icon={Globe}
					title={t('radar.no_countries', 'No country spaces yet')}
					body={t('radar.no_countries_body', 'Country spaces will appear here once they are created.')}
				/>
			{:else}
				<div class="space-y-1.5">
					<!-- Followed countries first -->
					{#each followedCountries as space (space.id)}
						<div
							class="flex items-center justify-between gap-3 rounded-[var(--radius)] border border-[var(--color-primary)]/30 bg-[var(--color-primary)]/5 px-3 py-2"
						>
							<a
								href={`/spaces/${space.slug}`}
								class="flex min-w-0 flex-1 items-center gap-2 text-sm font-medium text-[var(--color-text)] hover:text-[var(--color-primary)]"
							>
								<Pin size={14} class="shrink-0 text-[var(--color-primary)]" />
								<span class="truncate">{space.name}</span>
								{#if space.follower_count}
									<span class="text-xs font-normal text-[var(--color-muted)]">· {space.follower_count}</span>
								{/if}
							</a>
							{#if authed}
								<button
									type="button"
									onclick={() => toggleFollow(space)}
									disabled={pendingSpaceId === space.id}
									aria-busy={pendingSpaceId === space.id}
									class="shrink-0 rounded-full border border-[var(--color-border)] px-3 py-1 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
								>
									{#if pendingSpaceId === space.id}
										<Loader2 size={12} class="animate-spin" />
									{:else}
										{t('radar.unfollow', 'Unfollow')}
									{/if}
								</button>
							{/if}
						</div>
					{/each}

					<!-- Remaining countries -->
					{#each browseCountries as space (space.id)}
						<div
							class="group flex items-center justify-between gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 transition-colors hover:border-[var(--color-primary)]/40"
						>
							<a
								href={`/spaces/${space.slug}`}
								class="flex min-w-0 flex-1 items-center gap-2 text-sm text-[var(--color-text)] hover:text-[var(--color-primary)]"
							>
								<Globe size={14} class="shrink-0 text-[var(--color-muted)]" />
								<span class="truncate">{space.name}</span>
								{#if space.follower_count}
									<span class="text-xs font-normal text-[var(--color-muted)]">· {space.follower_count}</span>
								{/if}
							</a>
							{#if authed}
								<button
									type="button"
									onclick={() => toggleFollow(space)}
									disabled={pendingSpaceId === space.id}
									aria-busy={pendingSpaceId === space.id}
									class="shrink-0 rounded-full bg-[var(--color-primary)] px-3 py-1 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
								>
									{#if pendingSpaceId === space.id}
										<Loader2 size={12} class="animate-spin" />
									{:else}
										{t('radar.follow', 'Follow')}
									{/if}
								</button>
							{:else}
								<a
									href={`/spaces/${space.slug}`}
									class="flex shrink-0 items-center gap-0.5 text-xs text-[var(--color-muted)] hover:text-[var(--color-primary)]"
								>
									{t('common.view', 'View')}
									<ChevronRight size={12} />
								</a>
							{/if}
						</div>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
</div>

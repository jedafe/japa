<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto, invalidate } from '$app/navigation';
	import { t } from '$lib/i18n';
	import { Radar, Pin, Loader2, Lock, Globe, ChevronRight } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import { requireAuthOrRedirect } from '$lib/auth/guest';
	import RadarItemRow from '$lib/components/RadarItemRow.svelte';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import type { GeoCountry, RadarItem } from '$lib/types';
	import type { PageData } from './$types';
import { safeJsonLdScript } from '$lib/utils/safeHtml';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('radar.title', 'Japa Radar — JapaConnect'));
	const description = $derived(
		t('radar.description', 'Track scholarships and opportunities across the countries you follow.')
	);

	const authed = $derived(data.authed);
	const intelUnavailable = $derived(data.intelUnavailable);

	// Geo-aware filter state — when ?space_id= or ?dest= is present, the
	// page shows the filtered feed instead of the followed-countries feed.
	const hasGeoFilter = $derived(data.hasGeoFilter);
	const geoItems = $derived<RadarItem[]>(data.geoItems);
	const selectedSpaceId = $derived(data.spaceId);
	const selectedDest = $derived(data.dest);

	// Builds the radar URL for the country selector.
	function radarUrl(spaceId?: string): string {
		const p = new URLSearchParams();
		if (spaceId) p.set('space_id', spaceId);
		const qs = p.toString();
		return qs ? `?${qs}` : '?';
	}

	// The selected country name (for the filter badge). Resolved from the
	// countries list by space_id, or from the dest param.
	const selectedCountryName = $derived.by(() => {
		if (selectedSpaceId) {
			const space = countries.find((c) => c.id === selectedSpaceId);
			return space?.name ?? selectedSpaceId;
		}
		if (selectedDest) return selectedDest;
		return '';
	});

	// Followed feed items + country browse list (SSR-seeded).
	const followedItems = $derived<RadarItem[]>(data.followedItems);
	const countries = $derived<GeoCountry[]>(data.countries);

	// Followed space IDs — seeded from SSR, mutated optimistically on toggle.
	let followedSpaceIds = $state<Set<string>>(untrack(() => new Set(data.followedSpaceIds ?? [])));

	// Group the followed feed by country so each followed country renders as
	// a pinned section with its items beneath. Uses allFollowedItems (SSR +
	// load-more) so paginated items appear in the grouped view.
	const followedByCountry = $derived.by(() => {
		const map = new Map<string, RadarItem[]>();
		for (const item of allFollowedItems) {
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

	// Autocomplete fetchers — filter the full /geo/countries list client-side.
	// The "Browse countries" option (id: '') is always included at the top.
	function fetchCountries(q: string): Promise<AutocompleteItem[]> {
		const allLabel = t('radar.browse_countries', 'Browse countries');
		const query = q.toLowerCase().trim();
		const allItem: AutocompleteItem = { id: '', label: allLabel };
		if (query === '') {
			return Promise.resolve([allItem, ...data.countries.map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }))]);
		}
		const matchesAll = allLabel.toLowerCase().includes(query);
		const filtered = data.countries.filter(
			(c) => c.name.toLowerCase().includes(query) || c.geo_code.toLowerCase().includes(query)
		);
		const items = filtered.map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }));
		return Promise.resolve(matchesAll ? [allItem, ...items] : items);
	}
	function fetchCountriesInitial(): Promise<AutocompleteItem[]> {
		return fetchCountries('');
	}

	// "Load more" — appends the next page of followed items client-side.
	// The radar feed uses a non-standard envelope ({items, ...} per spec
	// §739) — the array is under `items`, not `data`, so api.get (which
	// reads body.data) cannot be used. We fetch directly and read `items`.
	let loadingMore = $state(false);
	let extraItems = $state<RadarItem[]>([]);
	let serverHasMore = $state(true);
	const allFollowedItems = $derived([...followedItems, ...extraItems]);
	const hasMore = $derived(serverHasMore && allFollowedItems.length >= data.pageSize);

	async function loadMore() {
		if (loadingMore || !authed) return;
		loadingMore = true;
		const nextOffset = allFollowedItems.length;
		try {
			const res = await api.getEnvelope<{ items: RadarItem[]; has_more?: boolean }>(
				`/intel/radar/followed?page_size=${data.pageSize}&page_offset=${nextOffset}`
			);
			const env = res.envelope as { items?: RadarItem[]; has_more?: boolean } | undefined;
			if (env?.items) {
				// Dedupe by id — rows promoted between page fetches can shift the
				// offset window and re-emit an item from the previous page.
				const seen = new Set(allFollowedItems.map((i) => i.id));
				const fresh = env.items.filter((i) => !seen.has(i.id));
				extraItems = [...extraItems, ...fresh];
			}
			if (env && env.has_more === false) {
				serverHasMore = false;
			}
		} catch {
			// network error — keep existing items
		}
		loadingMore = false;
	}

	// Follow / unfollow a country space. Calls POST/DELETE on the radar
	// follow endpoint, then invalidates the page load so the SSR data
	// (followed feed + followed_space_ids) re-fetches from the server.
	let pendingSpaceId = $state<string | null>(null);

	async function toggleFollow(space: GeoCountry) {
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
	{@html safeJsonLdScript(jsonLd)}
</svelte:head>

<div class="mx-auto w-full max-w-4xl">
	<!-- Header -->
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-4">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Radar size={20} /> {t('radar.heading', 'Japa Radar')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{description}</p>
	</div>

	{#if !intelUnavailable}
		<!-- Geo-aware country selector — the first input. Selecting a country
		     filters the radar feed to that country via ?space_id=. This matches
		     the geo-aware pattern used by other surfaces (§31 spaces, §32 feed).
		     BUG-012 fix. -->
		<div class="border-b border-[var(--color-border)] px-4 py-3">
			<div class="flex flex-wrap items-center gap-2">
				<Autocomplete
					placeholder={t('radar.browse_countries', 'Browse countries')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={fetchCountries}
					initialFetcher={fetchCountriesInitial}
					selectedId={selectedSpaceId || null}
					selectedLabel={selectedCountryName || null}
					onSelect={(item) => {
						if (item.id) goto(radarUrl(item.id), { keepFocus: true });
						else goto(radarUrl(), { keepFocus: true });
					}}
					onClear={() => goto(radarUrl(), { keepFocus: true })}
				/>
				{#if hasGeoFilter}
					<button
						type="button"
						onclick={() => goto(radarUrl(), { keepFocus: true })}
						class="shrink-0 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						{t('common.clear', 'Clear')}
					</button>
				{/if}
			</div>
		</div>
	{/if}

	{#if intelUnavailable}
		<div class="p-4">
			<EmptyState
				icon={Lock}
				title={t('radar.unavailable_title', 'Radar is currently unavailable')}
				body={t('radar.unavailable_body', 'The intelligence service is not configured. Check back soon.')}
			/>
		</div>
	{:else if hasGeoFilter}
		<!-- Geo-filtered feed — shows opportunities for the selected country
		     (via ?space_id= or ?dest=). This is public so guests can browse a
		     specific country's opportunities. BUG-012 fix. -->
		<div class="border-b border-[var(--color-border)] px-4 py-4">
			<h2 class="mb-3 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
				<Globe size={16} class="text-[var(--color-primary)]" />
				{selectedCountryName}
				{#if data.geoCount > 0}
					<span class="text-xs font-normal text-[var(--color-muted)]">({data.geoCount})</span>
				{/if}
			</h2>

			{#if geoItems.length === 0}
				<EmptyState
					icon={Radar}
					title={t('radar.empty_followed', 'No opportunities in your followed countries yet')}
					body={t('radar.empty_followed_body', 'Follow countries below to surface their latest scholarships here.')}
				/>
			{:else}
				<div class="space-y-2">
					{#each geoItems as item (item.id)}
						<RadarItemRow {item} />
					{/each}
				</div>
			{/if}
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

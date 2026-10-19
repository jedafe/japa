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
	import { servedMarkets, pinMarketsFirst } from '$lib/utils/geoMarkets';
import { safeJsonLdScript } from '$lib/utils/safeHtml';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('radar.title', 'Japa Radar — {site}'));
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

	// Followed feed items + country browse list (SSR-seeded).
	const followedItems = $derived<RadarItem[]>(data.followedItems);
	const countries = $derived<GeoCountry[]>(data.countries);

	// Builds the radar URL for the country selector.
	function radarUrl(spaceId?: string): string {
		if (!spaceId) return '/radar';
		return `?space_id=${encodeURIComponent(spaceId)}`;
	}

	// The selected country name (for the filter badge). Resolved from the
	// countries list by space_id, or from the dest param — which per the
	// geo-aware convention carries a country code or name, so resolve it
	// against the countries list (geo_code then name) before falling back
	// to the raw value.
	const selectedCountryName = $derived.by(() => {
		if (selectedSpaceId) {
			const space = countries.find((c) => c.id === selectedSpaceId);
			return space?.name ?? selectedSpaceId;
		}
		if (selectedDest) {
			const d = selectedDest.trim().toLowerCase();
			const space = countries.find(
				(c) => c.geo_code.toLowerCase() === d || c.slug === d || c.name.toLowerCase() === d
			);
			return space?.name ?? selectedDest;
		}
		return '';
	});

	// Domain filter (?item_type=) — URL-driven so deep links work and the
	// SSR feed stays the source of truth.
	const selectedItemType = $derived(data.itemType ?? '');
	const radarTypeOptions = ['', 'scholarship', 'job', 'visa', 'housing', 'remittance', 'cost_of_living'] as const;
	function typeUrl(t: string): string {
		const p = new URLSearchParams();
		if (selectedSpaceId) p.set('space_id', selectedSpaceId);
		else if (selectedDest) p.set('dest', selectedDest);
		if (t) p.set('item_type', t);
		const qs = p.toString();
		return qs ? `?${qs}` : '/radar';
	}

	// The resolved space for the geo filter — enables the follow button in
	// the geo view header. Resolves dest (geo_code/slug/name) too so
	// ?dest=united-states and ?dest=US follow the same space.
	const selectedSpace = $derived.by(() => {
		if (selectedSpaceId) {
			return countries.find((c) => c.id === selectedSpaceId) ?? null;
		}
		if (selectedDest) {
			const d = selectedDest.trim().toLowerCase();
			return (
				countries.find(
					(c) => c.geo_code.toLowerCase() === d || c.slug === d || c.name.toLowerCase() === d
				) ?? null
			);
		}
		return null;
	});

	// Followed space IDs — seeded from SSR, mutated optimistically on toggle,
	// re-synced to server truth whenever the load re-runs (e.g. after a
	// follow toggle invalidates the followed feed).
	let followedSpaceIds = $state<Set<string>>(untrack(() => new Set(data.followedSpaceIds ?? [])));
	let pendingSpaceId = $state<string | null>(null);

	$effect(() => {
		const server = new Set(data.followedSpaceIds ?? []);
		if (pendingSpaceId) return; // a toggle is in flight — keep optimistic state
		followedSpaceIds = server;
	});

	// Group the followed feed by country so each followed country renders as
	// a pinned section with its items beneath. Uses allFollowedItems (SSR +
	// load-more) so paginated items appear in the grouped view. Grouped by
	// country_space_id (spec §844) and sorted by the feed's counts_by_country
	// sidecar desc; items without a space link fall back to their country
	// text as the group key.
	const followedByCountry = $derived.by(() => {
		const map = new Map<string, { key: string; country: string; items: RadarItem[] }>();
		for (const item of allFollowedItems) {
			const key = item.space_id ?? `~${item.country}`;
			const entry = map.get(key);
			if (entry) entry.items.push(item);
			else map.set(key, { key, country: item.country, items: [item] });
		}
		const counts = data.followedCountsByCountry ?? {};
		return [...map.values()].sort(
			(a, b) => (counts[b.country] ?? b.items.length) - (counts[a.country] ?? a.items.length)
		);
	});

	// Countries the user follows — pinned to the top of the browse list.
	// ADR-064 served markets pin first, matching the country autocomplete.
	const followedCountries = $derived(
		pinMarketsFirst(
			countries.filter((c) => followedSpaceIds.has(c.id)),
			servedMarkets(data.siteSettings),
			(c) => c.geo_code
		)
	);
	const browseCountries = $derived(
		pinMarketsFirst(
			countries.filter((c) => !followedSpaceIds.has(c.id)),
			servedMarkets(data.siteSettings),
			(c) => c.geo_code
		)
	);

	// Autocomplete fetchers — filter the full /geo/countries list client-side.
	// The "Browse countries" option (id: '') is always included at the top.
	// ADR-064 curated markets (public settings) — pinned first; every
	// seeded country stays selectable (pin-first, not restricted).
	const radarCountries = $derived(
		pinMarketsFirst(data.countries, servedMarkets(data.siteSettings), (c) => c.geo_code)
	);

	function fetchCountries(q: string): Promise<AutocompleteItem[]> {
		const allLabel = t('radar.browse_countries', 'Browse countries');
		const query = q.toLowerCase().trim();
		const allItem: AutocompleteItem = { id: '', label: allLabel };
		if (query === '') {
			return Promise.resolve([allItem, ...radarCountries.map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }))]);
		}
		const matchesAll = allLabel.toLowerCase().includes(query);
		const filtered = radarCountries.filter(
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
	let serverHasMore = $state(untrack(() => data.followedHasMore));
	let followedCursor = $state(untrack(() => data.followedNextCursor));
	const allFollowedItems = $derived([...followedItems, ...extraItems]);
	const hasMore = $derived(serverHasMore && allFollowedItems.length >= data.pageSize);

	// Reset load-more accumulators whenever the SSR data is refreshed
	// (follow toggles invalidate the load, which returns a fresh page 1 —
	// stale extras would duplicate/ghost items). Also applies to the
	// geo-filtered feed below.
	let geoExtraItems = $state<RadarItem[]>([]);
	let geoServerHasMore = $state(untrack(() => data.geoHasMore));
	let geoCursor = $state(untrack(() => data.geoNextCursor));
	let loadingGeoMore = $state(false);
	const allGeoItems = $derived([...geoItems, ...geoExtraItems]);
	const geoHasMore = $derived(geoServerHasMore && allGeoItems.length >= data.pageSize);

	$effect(() => {
		data.followedItems;
		extraItems = [];
		serverHasMore = data.followedHasMore;
		followedCursor = data.followedNextCursor;
	});
	$effect(() => {
		data.geoItems;
		geoExtraItems = [];
		geoServerHasMore = data.geoHasMore;
		geoCursor = data.geoNextCursor;
	});

	// Compact per-type breakdown of the loaded page (spec §739
	// counts_by_type sidecar) — shown under the feed heading.
	function typeBreakdown(counts: Record<string, number> | undefined): string {
		if (!counts) return '';
		return Object.entries(counts)
			.filter(([, n]) => n > 0)
			.sort((a, b) => b[1] - a[1])
			.map(([type, n]) => `${t(`radar.type.${type}`, type.replaceAll('_', ' '))}: ${n}`)
			.join(' · ');
	}

	async function loadMoreGeo() {
		// Keyset cursor pagination per spec §848 — the cursor already encodes
		// the SSR page offset, so no page_offset math is needed and newly
		// promoted rows can't shift the window (no client-side dedupe gaps).
		if (loadingGeoMore || !geoCursor) return;
		loadingGeoMore = true;
		try {
			const params = new URLSearchParams();
			params.set('page_size', String(data.pageSize));
			params.set('cursor', geoCursor);
			if (selectedSpaceId) params.set('space_id', selectedSpaceId);
			else if (selectedSpace) params.set('space_id', selectedSpace.id);
			else if (selectedDest) params.set('country', selectedDest);
			if (data.itemType) params.set('item_type', data.itemType);
			const res = await api.getEnvelope<{ items: RadarItem[]; has_more?: boolean; next_cursor?: string }>(
				`/intel/radar?${params.toString()}`
			);
			const env = res.envelope as { items?: RadarItem[]; has_more?: boolean; next_cursor?: string } | undefined;
			if (env?.items) {
				const seen = new Set(allGeoItems.map((i) => i.id));
				geoExtraItems = [...geoExtraItems, ...env.items.filter((i) => !seen.has(i.id))];
			}
			if (env?.next_cursor) geoCursor = env.next_cursor;
			if (env && env.has_more === false) geoServerHasMore = false;
		} catch {
			// network error — keep existing items
		}
		loadingGeoMore = false;
	}

	async function loadMore() {
		if (loadingMore || !authed || !followedCursor) return;
		loadingMore = true;
		const params = new URLSearchParams();
		params.set('page_size', String(data.pageSize));
		params.set('cursor', followedCursor);
		if (data.itemType) params.set('item_type', data.itemType);
		try {
			const res = await api.getEnvelope<{ items: RadarItem[]; has_more?: boolean; next_cursor?: string }>(
				`/intel/radar/followed?${params.toString()}`
			);
			const env = res.envelope as { items?: RadarItem[]; has_more?: boolean; next_cursor?: string } | undefined;
			if (env?.items) {
				// Dedupe by id — belt-and-braces; the keyset cursor already makes
				// overlap impossible, but a stale response can't hurt.
				const seen = new Set(allFollowedItems.map((i) => i.id));
				const fresh = env.items.filter((i) => !seen.has(i.id));
				extraItems = [...extraItems, ...fresh];
			}
			if (env?.next_cursor) followedCursor = env.next_cursor;
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

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 48rem)/2))">
	<div class="mx-auto w-full max-w-3xl p-4">
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

			<!-- Domain filter — ?item_type= per catalog. Only shown when there
			     is a feed to filter: the followed feed (authed) or the
			     geo-filtered feed (anyone). -->
			{#if authed || hasGeoFilter}
				<div class="mt-2.5 flex flex-wrap items-center gap-1.5" role="group" aria-label={t('radar.filter_type', 'Filter by type')}>
					{#each radarTypeOptions as opt (opt)}
						<button
							type="button"
							onclick={() => goto(typeUrl(opt), { keepFocus: true })}
							aria-pressed={selectedItemType === opt}
							class="rounded-full px-2.5 py-1 text-xs font-medium transition-colors {selectedItemType === opt
								? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
								: 'border border-[var(--color-border)] text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]'}"
						>
							{opt === '' ? t('radar.type.all', 'All') : t(`radar.type.${opt}`, opt.replaceAll('_', ' '))}
						</button>
					{/each}
				</div>
			{/if}
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
				{#if allGeoItems.length > 0}
					<span class="text-xs font-normal text-[var(--color-muted)]">({allGeoItems.length})</span>
				{/if}
				{#if typeBreakdown(data.geoCountsByType)}
					<span class="text-xs font-normal normal-case text-[var(--color-muted)]">
						{typeBreakdown(data.geoCountsByType)}
					</span>
				{/if}
				{#if authed && selectedSpace}
					<button
						type="button"
						onclick={() => toggleFollow(selectedSpace!)}
						disabled={pendingSpaceId === selectedSpace!.id}
						aria-busy={pendingSpaceId === selectedSpace!.id}
						class="ml-auto shrink-0 rounded-full px-3 py-1 text-xs font-medium transition-colors disabled:opacity-50 {followedSpaceIds.has(selectedSpace!.id)
							? 'border border-[var(--color-border)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'
							: 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]'}"
					>
						{#if pendingSpaceId === selectedSpace!.id}
							<Loader2 size={12} class="inline animate-spin" />
						{:else}
							{followedSpaceIds.has(selectedSpace!.id) ? t('radar.unfollow', 'Unfollow') : t('radar.follow', 'Follow')}
						{/if}
					</button>
				{/if}
			</h2>

			{#if allGeoItems.length === 0}
				<EmptyState
					icon={Radar}
					title={t('radar.empty_geo', 'No opportunities for {country} yet', { country: selectedCountryName })}
					body={t('radar.empty_geo_body', "We'll surface new items here as sources publish them.", { country: selectedCountryName })}
				/>
			{:else}
				<div class="space-y-2">
					{#each allGeoItems as item (item.id)}
						<RadarItemRow {item} />
					{/each}
				</div>

				{#if geoHasMore}
					<div class="mt-4 flex justify-center">
						<button
							type="button"
							onclick={loadMoreGeo}
							disabled={loadingGeoMore}
							aria-busy={loadingGeoMore}
							class="inline-flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
						>
							{#if loadingGeoMore}
								<Loader2 size={16} class="animate-spin" />
							{/if}
							{t('radar.load_more', 'Load more')}
						</button>
					</div>
				{/if}
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
					{#if typeBreakdown(data.followedCountsByType)}
						<span class="ml-auto text-xs font-normal normal-case text-[var(--color-muted)]">
							{typeBreakdown(data.followedCountsByType)}
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
						{#each followedByCountry as group (group.key)}
							<section>
								<h3 class="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
									<Globe size={12} /> {group.country || t('radar.uncategorized', 'Uncategorized')}
									<span class="font-normal normal-case text-[var(--color-muted)]">· {group.items.length}</span>
								</h3>
								<div class="space-y-2">
									{#each group.items as item (item.id)}
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
</div>

<script lang="ts">
	/**
	 * Global tab feed — §126 M5 remote fediverse timeline + tag discovery.
	 *
	 * Cursor-paginated (next_cursor keyset) unlike the offset-paginated local
	 * feeds in FeedList, so it gets its own list component with the same
	 * IntersectionObserver infinite-scroll behaviour.
	 *
	 * Tag discovery combines three mechanisms: ?tag= filters stored posts,
	 * tag subscriptions keep matching remote posts flowing in (relay /
	 * Follow{Hashtag} delivery), and the live search fans out to remote
	 * instances' public tag timelines for a preview.
	 *
	 * Ref: japaconnect-engineering/sections/126-activitypub-federation.md (M5)
	 */
	import { untrack } from 'svelte';
	import { goto, invalidateAll } from '$app/navigation';
	import { navigating, page } from '$app/state';
	import { AtSign, Hash, LoaderCircle, Satellite, Search, TriangleAlert, X } from 'lucide-svelte';
	import { api } from '#lib/api/client.js';
	import { EmptyState, LoadingSkeleton } from '#lib/components/ui/index.js';
	import { t } from '#lib/i18n/index.js';
	import type {
		RemoteTagSearchResult,
		RemoteTagSubscription,
		RemoteTimelineItem,
		RemoteTimelinePage
	} from '#lib/types/index.js';
	import RemotePostCard from './RemotePostCard.svelte';

	interface Props {
		items: RemoteTimelineItem[];
		nextCursor: string | null;
		/** Backend answered 404 not_federated — the member hasn't opted in
		 *  (or the feature flag is off): render the enable CTA, not an error. */
		notFederated: boolean;
		loadError: boolean;
		/** Active ?tag= filter from the URL (drives live search client-side). */
		tag?: string;
		/** ?domain= pin — live search hits only this remote instance. */
		domain?: string;
		/** Member's tag ingest warrants from the SSR load. */
		subscriptions?: RemoteTagSubscription[];
	}
	let {
		items: initial,
		nextCursor: initialCursor,
		notFederated,
		loadError,
		tag = '',
		domain = '',
		subscriptions = []
	}: Props = $props();

	const PAGE_SIZE = 20;

	let items = $state<RemoteTimelineItem[]>(untrack(() => initial));
	let nextCursor = $state<string | null>(untrack(() => initialCursor));
	let loading = $state(false);
	let sentinel = $state<HTMLElement | null>(null);
	let clientError = $state<string | null>(null);

	let tagInput = $state(untrack(() => tag));
	let liveItems = $state<RemoteTimelineItem[]>([]);
	let liveErrors = $state<{ domain: string; message: string }[]>([]);
	let liveApiError = $state<string | null>(null);
	let liveLoading = $state(false);
	let busySub = $state<string | null>(null);
	let subMessage = $state<string | null>(null);

	const atEnd = $derived(!nextCursor);

	// Live results deduped against stored posts and themselves (two
	// instances can surface the same boosted status under different ids —
	// ap_id is the stable remote identity when present).
	const storedIds = $derived(new Set(items.map((i) => i.ap_id || i.id)));
	const liveDeduped = $derived.by(() => {
		const seen = new Set(storedIds);
		return liveItems.filter((i) => {
			const k = i.ap_id || i.url || i.id;
			if (!k || seen.has(k)) return false;
			seen.add(k);
			return true;
		});
	});
	const liveByDomain = $derived.by(() => {
		const groups = new Map<string, RemoteTimelineItem[]>();
		for (const item of liveDeduped) {
			const d = item.actor_domain || 'remote';
			const g = groups.get(d) ?? [];
			g.push(item);
			groups.set(d, g);
		}
		return [...groups.entries()];
	});

	const isSubscribed = $derived(
		(domain: string) => subscriptions.some((s) => s.tag === tag.toLowerCase() && s.domain === domain)
	);

	// Re-seed on a new SSR payload (tab/tag switch re-runs load) — replace,
	// never append.
	$effect(() => {
		const seed = initial;
		const cursor = initialCursor;
		untrack(() => {
			items = seed;
			nextCursor = cursor;
			tagInput = domain ? `${tag}@${domain}` : tag;
			clientError = null;
			loading = false;
		});
	});

	// Live remote search — fires per distinct tag in the browser only.
	// Stale results are dropped via a run token so a fast re-search never
	// renders the previous tag's page.
	let liveRun = 0;
	$effect(() => {
		const q = tag.trim().toLowerCase();
		if (!q) {
			liveItems = [];
			liveErrors = [];
			liveApiError = null;
			return;
		}
		const run = ++liveRun;
		liveLoading = true;
		liveApiError = null;
		const pin = domain ? `&domain=${encodeURIComponent(domain)}` : '';
		api.get<RemoteTagSearchResult>(`/me/federation/search?tag=${encodeURIComponent(q)}${pin}`)
			.then((res) => {
				if (run !== liveRun) return;
				if (res.error) {
					liveItems = [];
					liveErrors = [];
					liveApiError = res.error.message ?? t('error.500_body');
					return;
				}
				liveItems = res.data?.items ?? [];
				liveErrors = res.data?.errors ?? [];
			})
			.finally(() => {
				if (run === liveRun) liveLoading = false;
			});
	});

	$effect(() => {
		const el = sentinel;
		if (!el || atEnd) return;
		const obs = new IntersectionObserver(
			(entries) => {
				if (entries[0]?.isIntersecting) loadMore();
			},
			{ rootMargin: '600px' }
		);
		obs.observe(el);
		return () => obs.disconnect();
	});

	function timelineQs(cursor: string | null) {
		const params = new URLSearchParams({ limit: String(PAGE_SIZE) });
		if (cursor) params.set('cursor', cursor);
		if (tag) params.set('tag', tag);
		return params.toString();
	}

	async function loadMore() {
		if (loading || !nextCursor) return;
		loading = true;
		clientError = null;
		const res = await api.get<RemoteTimelinePage>(`/me/federation/timeline?${timelineQs(nextCursor)}`);
		if (res.error) {
			clientError = res.error.message ?? t('error.500_body');
			loading = false;
			return;
		}
		const batch = res.data?.items ?? [];
		// Dedup by row id — a post can shift across the keyset boundary
		// between page fetches (§6.3 each_key_duplicate).
		const seen = new Set(items.map((i) => i.id));
		const fresh = batch.filter((i) => !seen.has(i.id) && !!seen.add(i.id));
		items = [...items, ...fresh];
		nextCursor = res.data?.next_cursor ?? null;
		loading = false;
	}

	// Tag filter lives in the URL (?tab=global&tag=&domain=) — same
	// mechanism as the tab switcher, so searches are shareable and SSR
	// covers them. The input accepts tag@domain to pin the live search
	// to one instance (mirroring the user@domain follow syntax).
	function applyTag(raw: string) {
		const url = new URL(page.url.href);
		url.searchParams.set('tab', 'global');
		const clean = raw.trim().replace(/^#/, '');
		const at = clean.indexOf('@');
		const q = at > 0 ? clean.slice(0, at) : clean;
		const d = at > 0 ? clean.slice(at + 1).trim().toLowerCase() : '';
		if (q) url.searchParams.set('tag', q);
		else url.searchParams.delete('tag');
		if (d) url.searchParams.set('domain', d);
		else url.searchParams.delete('domain');
		goto(`${url.pathname}${url.search}`, { reset: false });
	}

	async function subscribeTag(subDomain: string) {
		const q = tag.trim().toLowerCase();
		if (!q || busySub) return;
		busySub = subDomain || '*';
		subMessage = null;
		const res = await api.post('/me/federation/tag-subscriptions', { tag: q, domain: subDomain });
		busySub = null;
		if (res.error) {
			subMessage = res.error.message ?? t('error.500_body');
			return;
		}
		await invalidateAll();
	}

	async function unsubscribeTag(id: string) {
		if (busySub) return;
		busySub = id;
		subMessage = null;
		const res = await api.delete(`/me/federation/tag-subscriptions/${id}`);
		busySub = null;
		if (res.error) {
			subMessage = res.error.message ?? t('error.500_body');
			return;
		}
		await invalidateAll();
	}
</script>

{#if loadError}
	<EmptyState icon={TriangleAlert} title={t('error.500_title')} body={t('error.500_body')} />
{:else if notFederated}
	<EmptyState
		icon={AtSign}
		title={t('feed.global.enable_title', 'Federation is off for your account')}
		body={t('feed.global.enable_body', 'Enable federation to follow fediverse accounts — their posts will show up here.')}
		action={{ label: t('feed.global.enable_cta', 'Open federation settings'), href: '/settings/federation' }}
	/>
{:else}
	<!-- §126 tag discovery: search → filter + live remote preview → subscribe -->
	<div class="mb-4">
		<form
			class="flex items-center gap-2"
			onsubmit={(e) => { e.preventDefault(); applyTag(tagInput); }}
		>
			<div class="relative min-w-0 flex-1">
				<Search size={15} class="pointer-events-none absolute start-3 top-1/2 -translate-y-1/2 text-[var(--color-muted)]" />
				<input
					type="text"
					bind:value={tagInput}
					placeholder={t('feed.global.search_placeholder', 'Search a fediverse hashtag…')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-2 pe-3 ps-9 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)]"
				/>
			</div>
			<button
				type="submit"
				class="shrink-0 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:opacity-90"
			>
				{t('feed.global.search_action', 'Search')}
			</button>
		</form>

		{#if subscriptions.length > 0}
			<div class="mt-2 flex flex-wrap items-center gap-1.5">
				<span class="text-[11px] font-medium uppercase tracking-wide text-[var(--color-muted)]">{t('feed.global.subs_title', 'Following tags')}</span>
				{#each subscriptions as sub (sub.id)}
					<span class="inline-flex items-center gap-1 rounded-full border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs text-[var(--color-text)]">
						<Hash size={11} />{sub.tag}{#if sub.domain}<span class="text-[var(--color-muted)]">@{sub.domain}</span>{/if}
						<button
							type="button"
							disabled={busySub === sub.id}
							onclick={() => unsubscribeTag(sub.id)}
							class="ms-0.5 text-[var(--color-muted)] hover:text-[var(--color-danger)] disabled:opacity-50"
							aria-label={t('feed.global.unsubscribe', 'Unsubscribe')}
						><X size={11} /></button>
					</span>
				{/each}
			</div>
		{/if}
		{#if subMessage}
			<p class="mt-1.5 text-xs text-[var(--color-danger)]">{subMessage}</p>
		{/if}
	</div>

	{#if tag}
		<div class="mb-3 flex flex-wrap items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-xs">
			<span class="flex items-center gap-1 font-medium text-[var(--color-text)]"><Hash size={12} />{tag}{#if domain}<span class="text-[var(--color-muted)]">@{domain}</span>{/if}</span>
			<button type="button" onclick={() => applyTag('')} class="text-[var(--color-muted)] hover:text-[var(--color-text)] hover:underline">
				{t('feed.global.tag_clear', 'Clear filter')}
			</button>
			<span class="ms-auto flex items-center gap-2">
				{#if domain && !isSubscribed(domain)}
					<button
						type="button"
						disabled={busySub !== null}
						onclick={() => subscribeTag(domain)}
						class="flex items-center gap-1 rounded-[var(--radius)] bg-[var(--color-primary)] px-2.5 py-1 font-medium text-[var(--color-primary-contrast)] hover:opacity-90 disabled:opacity-50"
					>
						<Satellite size={11} />{t('feed.global.subscribe_domain', 'Follow #{tag} on {domain}', { tag, domain })}
					</button>
				{/if}
				{#if !isSubscribed('')}
					<button
						type="button"
						disabled={busySub !== null}
						onclick={() => subscribeTag('')}
						class="flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-2.5 py-1 font-medium text-[var(--color-text)] hover:bg-[var(--color-surface)] disabled:opacity-50"
					>
						{t('feed.global.subscribe_everywhere', 'Follow tag everywhere')}
					</button>
				{:else}
					<span class="text-[var(--color-muted)]">{t('feed.global.subscribed_everywhere', 'Following on all reachable instances')}</span>
				{/if}
			</span>
		</div>
	{/if}

	{#if items.length === 0 && !tag}
		<EmptyState
			icon={AtSign}
			title={t('feed.global.empty_title', 'Nothing from the fediverse yet')}
			body={t('feed.global.empty_body', 'Follow fediverse accounts from settings and their public posts will appear in this feed.')}
			action={{ label: t('feed.global.manage_following', 'Manage federation'), href: '/settings/federation' }}
		/>
	{:else if items.length === 0 && tag && !liveLoading && liveDeduped.length === 0}
		<EmptyState
			icon={Hash}
			title={t('feed.global.tag_empty_title', 'No posts for #{tag}', { tag })}
			body={t('feed.global.tag_empty_body', 'Subscribe to this tag and matching public posts will land here as they arrive.')}
		/>
	{:else}
		{#if navigating.to}
			<LoadingSkeleton variant="card" count={3} class="p-2" />
		{/if}

		<div class="flex flex-col gap-3">
			{#each items as item (item.id)}
				<RemotePostCard {item} onTag={applyTag} />
			{/each}
		</div>

		{#if !atEnd}
			<div bind:this={sentinel} class="h-8" aria-hidden="true"></div>
			{#if loading}<LoadingSkeleton variant="lines" count={2} class="p-4" />{/if}
		{:else if !tag}
			<p class="py-6 text-center text-xs text-[var(--color-muted)]">{t('feed.end', "You're all caught up")}</p>
		{/if}
	{/if}

	<!-- Live remote results — fetched per search, never persisted -->
	{#if tag}
		<div class="mt-5">
			<h2 class="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
				<Satellite size={12} />
				{t('feed.global.live_title', 'Live from the fediverse')}
			</h2>
			{#if liveLoading}
				<div class="mt-2 flex items-center gap-2 text-xs text-[var(--color-muted)]">
					<LoaderCircle size={13} class="animate-spin" />
					{t('feed.global.live_loading', 'Asking remote instances…')}
				</div>
			{:else if liveApiError}
				<p class="mt-2 text-xs text-[var(--color-danger)]">{liveApiError}</p>
			{:else if liveByDomain.length === 0}
				<p class="mt-2 text-xs text-[var(--color-muted)]">{t('feed.global.live_empty', 'No live results from reachable instances.')}</p>
			{:else}
				{#each liveByDomain as [domain, group] (domain)}
					<div class="mt-3">
						<div class="mb-2 flex items-center gap-2">
							<span class="text-xs font-medium text-[var(--color-text)]">{domain}</span>
							<span class="text-[10px] text-[var(--color-muted)]">{group.length} {t('feed.global.live_posts', 'posts')}</span>
							{#if !isSubscribed(domain)}
								<button
									type="button"
									disabled={busySub !== null}
									onclick={() => subscribeTag(domain)}
									class="ms-auto rounded-[var(--radius)] border border-[var(--color-border)] px-2 py-0.5 text-[11px] font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
								>
									{t('feed.global.subscribe_domain', 'Follow #{tag} on {domain}', { tag, domain })}
								</button>
							{:else}
								<span class="ms-auto text-[11px] text-[var(--color-muted)]">{t('feed.global.subscribed_domain', 'Following')}</span>
							{/if}
						</div>
						<div class="flex flex-col gap-3">
							{#each group as item, i (item.ap_id || item.url || i)}
								<RemotePostCard {item} onTag={applyTag} />
							{/each}
						</div>
					</div>
				{/each}
			{/if}
			{#if liveErrors.length > 0}
				<p class="mt-2 text-[11px] text-[var(--color-muted)]">
					{t('feed.global.live_errors', 'Unreachable: {domains}', { domains: liveErrors.map((e) => e.domain).join(', ') })}
				</p>
			{/if}
		</div>
	{/if}

	{#if clientError}
		<div class="mt-4 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 text-center">
			<p class="text-sm text-[var(--color-text)]">{clientError}</p>
			<button
				type="button"
				disabled={loading}
				onclick={loadMore}
				class="mt-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:opacity-90 disabled:opacity-50"
			>
				{t('common.retry')}
			</button>
		</div>
	{/if}
{/if}

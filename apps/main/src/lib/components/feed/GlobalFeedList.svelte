<script lang="ts">
	/**
	 * Global tab feed — §126 M5 remote fediverse timeline.
	 *
	 * Cursor-paginated (next_cursor keyset) unlike the offset-paginated local
	 * feeds in FeedList, so it gets its own list component with the same
	 * IntersectionObserver infinite-scroll behaviour.
	 *
	 * Ref: japaconnect-engineering/sections/126-activitypub-federation.md (M5)
	 */
	import { untrack } from 'svelte';
	import { navigating } from '$app/state';
	import { AtSign, TriangleAlert } from 'lucide-svelte';
	import { api } from '#lib/api/client.js';
	import { EmptyState, LoadingSkeleton } from '#lib/components/ui/index.js';
	import { t } from '#lib/i18n/index.js';
	import type { RemoteTimelineItem, RemoteTimelinePage } from '#lib/types/index.js';
	import RemotePostCard from './RemotePostCard.svelte';

	interface Props {
		items: RemoteTimelineItem[];
		nextCursor: string | null;
		/** Backend answered 404 not_federated — the member hasn't opted in
		 *  (or the feature flag is off): render the enable CTA, not an error. */
		notFederated: boolean;
		loadError: boolean;
	}
	let { items: initial, nextCursor: initialCursor, notFederated, loadError }: Props = $props();

	const PAGE_SIZE = 20;

	let items = $state<RemoteTimelineItem[]>(untrack(() => initial));
	let nextCursor = $state<string | null>(untrack(() => initialCursor));
	let loading = $state(false);
	let sentinel = $state<HTMLElement | null>(null);
	let clientError = $state<string | null>(null);

	const atEnd = $derived(!nextCursor);

	// Re-seed on a new SSR payload (tab switch re-runs load) — replace,
	// never append.
	$effect(() => {
		const seed = initial;
		const cursor = initialCursor;
		untrack(() => {
			items = seed;
			nextCursor = cursor;
			clientError = null;
			loading = false;
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

	async function loadMore() {
		if (loading || !nextCursor) return;
		loading = true;
		clientError = null;
		const res = await api.get<RemoteTimelinePage>(
			`/me/federation/timeline?limit=${PAGE_SIZE}&cursor=${encodeURIComponent(nextCursor)}`
		);
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
{:else if items.length === 0}
	<EmptyState
		icon={AtSign}
		title={t('feed.global.empty_title', 'Nothing from the fediverse yet')}
		body={t('feed.global.empty_body', 'Follow fediverse accounts from settings and their public posts will appear in this feed.')}
		action={{ label: t('feed.global.manage_following', 'Manage federation'), href: '/settings/federation' }}
	/>
{:else}
	{#if navigating.to}
		<LoadingSkeleton variant="card" count={3} class="p-2" />
	{/if}

	<div class="flex flex-col gap-3">
		{#each items as item (item.id)}
			<RemotePostCard {item} />
		{/each}
	</div>

	{#if !atEnd}
		<div bind:this={sentinel} class="h-8" aria-hidden="true"></div>
		{#if loading}<LoadingSkeleton variant="lines" count={2} class="p-4" />{/if}
	{:else}
		<p class="py-6 text-center text-xs text-[var(--color-muted)]">{t('feed.end', "You're all caught up")}</p>
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

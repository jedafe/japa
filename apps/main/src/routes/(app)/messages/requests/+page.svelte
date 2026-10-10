<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { EmptyState, AutocompleteSelect, ListPageBar } from '$lib/components/ui';
	import MessagesTabs from '$lib/components/messages/MessagesTabs.svelte';
	import { UserX, Search, Loader2 } from 'lucide-svelte';
	import type { DMRequest } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let requests = $state<DMRequest[]>(untrack(() => data.requests));
	let nextCursor = $state<string | null>(untrack(() => data.requestsCursor));
	let loadingMore = $state(false);
	let acting = $state<string | null>(null);
	let actionError = $state('');

	let searchValue = $state(untrack(() => data.query.q));
	let searchTimer: ReturnType<typeof setTimeout> | undefined;

	$effect(() => {
		requests = data.requests;
		nextCursor = data.requestsCursor;
		searchValue = data.query.q;
	});

	const title = $derived(t('messages.requests_title', 'Message requests — JapaConnect'));

	const sortOptions = [
		{ value: 'recent', label: t('messages.sort_recent', 'Most recent') },
		{ value: 'oldest', label: t('messages.sort_oldest', 'Oldest') },
		{ value: 'name', label: t('messages.sort_name', 'Name (A–Z)') }
	];
	const pageSizeOptions = [
		{ value: '10', label: '10' },
		{ value: '20', label: '20' },
		{ value: '30', label: '30' },
		{ value: '50', label: '50' }
	];

	function listHref(overrides: { q?: string; sort?: string; pageSize?: number } = {}): string {
		const params = new URLSearchParams();
		const q = overrides.q ?? data.query.q;
		const sort = overrides.sort ?? data.query.sort;
		const pageSize = overrides.pageSize ?? data.query.pageSize;
		if (q) params.set('q', q);
		if (sort && sort !== 'recent') params.set('sort', sort);
		if (pageSize && pageSize !== 30) params.set('page_size', String(pageSize));
		const qs = params.toString();
		return `/messages/requests${qs ? `?${qs}` : ''}`;
	}

	function onSearchInput(value: string) {
		searchValue = value;
		clearTimeout(searchTimer);
		searchTimer = setTimeout(() => {
			goto(listHref({ q: value.trim() }), { keepFocus: true, noScroll: true });
		}, 300);
	}

	function reqQueryString(): string {
		const params = new URLSearchParams();
		params.set('page_size', String(data.query.pageSize));
		if (data.query.q) params.set('q', data.query.q);
		if (data.query.sort !== 'recent') params.set('sort', data.query.sort);
		return params.toString();
	}

	async function loadMore() {
		if (loadingMore || !nextCursor) return;
		loadingMore = true;
		const res = await api.getEnvelope<DMRequest[]>(
			`/dm/requests?${reqQueryString()}&cursor=${encodeURIComponent(nextCursor)}`
		);
		loadingMore = false;
		const items = res.data ?? [];
		nextCursor = (res.envelope?.next_cursor as string | null | undefined) ?? null;
		if (items.length > 0) {
			const seen = new Set(requests.map((r) => r.id));
			requests = [...requests, ...items.filter((r) => !seen.has(r.id))];
		}
	}

	async function handleAction(req: DMRequest, action: 'accept' | 'decline' | 'block') {
		if (acting) return;
		actionError = '';
		acting = req.id;
		const res = await api.post<{ status: string; conversation_id?: string }>(
			`/dm/requests/${req.id}/${action}`
		);
		acting = null;
		if (res.error) {
			actionError =
				res.error.message ??
				t('messages.requests_action_error', 'The action could not be completed.');
			return;
		}

		requests = requests.filter((r) => r.id !== req.id);

		// Accept returns the conversation_id so we can open the thread.
		const conversationId = res.data?.conversation_id ?? req.conversation_id;
		if (action === 'accept' && conversationId) {
			await goto(`/messages?c=${conversationId}`);
		}
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={title} />
	<meta property="og:title" content={title} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<ListPageBar>
		<MessagesTabs value="requests" />
	</ListPageBar>

	<div class="mx-auto w-full max-w-2xl">
		<div class="flex flex-wrap items-center gap-2 px-4 py-3">
			<div class="relative min-w-0 flex-1">
				<Search
					size={15}
					class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-[var(--color-muted)]"
				/>
				<input
					type="search"
					value={searchValue}
					oninput={(e) => onSearchInput(e.currentTarget.value)}
					placeholder={t('messages.requests_search', 'Search requests')}
					aria-label={t('messages.requests_search', 'Search requests')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-1.5 pl-9 pr-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
				/>
			</div>
			<div class="w-40">
				<AutocompleteSelect
					options={sortOptions}
					value={data.query.sort}
					onValueChange={(v) => goto(listHref({ sort: v }), { noScroll: true })}
					ariaLabel={t('messages.sort_label', 'Sort requests')}
				/>
			</div>
		</div>

		{#if actionError}
			<p class="px-4 pb-2 text-sm text-[var(--color-danger)]">{actionError}</p>
		{/if}

		{#if data.loadError}
			<div class="p-4">
				<EmptyState
					icon={UserX}
					title={t('messages.requests_load_error', 'Could not load requests')}
				/>
			</div>
		{:else if requests.length === 0}
			<div class="p-4">
				<EmptyState icon={UserX} title={t('messages.no_requests', 'No pending requests')} />
			</div>
		{:else}
			<ul class="divide-y divide-[var(--color-border)]">
				{#each requests as req (req.id)}
					<li class="flex items-center gap-3 p-4">
						{#if req.requester_avatar_url}
							<img
								src={req.requester_avatar_url}
								alt=""
								class="h-10 w-10 shrink-0 rounded-full object-cover"
							/>
						{:else}
							<span
								class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]"
							>
								{req.requester_display_name.charAt(0).toUpperCase()}
							</span>
						{/if}
						<div class="min-w-0 flex-1">
							<p class="truncate text-sm font-medium text-[var(--color-text)]">
								{req.requester_display_name}
							</p>
							<a
								href="/u/{req.requester_username}"
								class="truncate text-xs text-[var(--color-muted)] hover:text-[var(--color-primary)]"
							>
								@{req.requester_username}
							</a>
							<p class="mt-1 truncate text-sm text-[var(--color-text-subtle)]">
								{req.preview_message}
							</p>
						</div>
						<div class="flex shrink-0 gap-2">
							<button
								type="button"
								onclick={() => handleAction(req, 'accept')}
								disabled={acting === req.id}
								class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
							>
								{t('messages.requests_accept', 'Accept')}
							</button>
							<button
								type="button"
								onclick={() => handleAction(req, 'decline')}
								disabled={acting === req.id}
								class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
							>
								{t('messages.requests_decline', 'Decline')}
							</button>
							<button
								type="button"
								onclick={() => handleAction(req, 'block')}
								disabled={acting === req.id}
								class="rounded-[var(--radius)] px-3 py-1.5 text-xs font-medium text-[var(--color-danger)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
							>
								{t('messages.requests_block', 'Block')}
							</button>
						</div>
					</li>
				{/each}
			</ul>

			<div class="flex items-center justify-between gap-2 border-t border-[var(--color-border)] px-4 py-2">
				{#if nextCursor}
					<button
						type="button"
						onclick={loadMore}
						disabled={loadingMore}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					>
						{#if loadingMore}<Loader2 size={12} class="animate-spin" />{/if}
						{t('messages.load_more', 'Load more')}
					</button>
				{:else}
					<span></span>
				{/if}
				<div class="w-20">
					<AutocompleteSelect
						options={pageSizeOptions}
						value={String(data.query.pageSize)}
						onValueChange={(v) => goto(listHref({ pageSize: Number(v) }), { noScroll: true })}
						ariaLabel={t('messages.page_size', 'Requests per page')}
						openUp
					/>
				</div>
			</div>
		{/if}
	</div>
</div>

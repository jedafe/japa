<script lang="ts">
	// Communications Hub — the /messages surface re-imagined as a unified
	// inbox: one stream of member DMs, org-inbox threads, DM requests,
	// service bookings, mentorship bookings, support tickets and Japa AI
	// conversations, with a thread pane and a conversation-details rail.
	//
	// Ref: japaconnect-engineering/sections/48-direct-messages.md (DMs),
	// sections/38-ticket-system.md, sections/111-service-packages-bookings.md,
	// sections/102-mentorship-matching.md, adr/ADR-130-unified-japa-ai.md
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import { untrack, onMount } from 'svelte';
	import { t } from '#lib/i18n/index.js';
	import { goto } from '$app/navigation';
	import {
		MessageSquareOff,
		Search,
		Loader2,
		Plus,
		Settings,
		ExternalLink
	} from 'lucide-svelte';
	import { api } from '#lib/api/client.js';
	import {
		EmptyState,
		LiteralSuggest,
		Modal,
		AutocompleteSelect,
		ListPageBar
	} from '#lib/components/ui/index.js';
	import MessagesTabs from '#lib/components/messages/MessagesTabs.svelte';
	import Autocomplete from '#lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '#lib/components/create/Autocomplete.svelte';
	import HubListItem from '#lib/components/hub/HubListItem.svelte';
	import DmThreadPane from '#lib/components/hub/DmThreadPane.svelte';
	import TicketThreadPane from '#lib/components/hub/TicketThreadPane.svelte';
	import AiThreadPane from '#lib/components/hub/AiThreadPane.svelte';
	import RequestPane from '#lib/components/hub/RequestPane.svelte';
	import ConversationDetailsPanel from '#lib/components/hub/ConversationDetailsPanel.svelte';
	import BookingDetailPane from '#lib/components/messages/BookingDetailPane.svelte';
	import { realtime, type DMMessageEvent } from '#lib/realtime/realtime.svelte.js';
	import {
		dmToHubItem,
		requestToHubItem,
		bookingToHubItem,
		ticketToHubItem,
		aiToHubItem,
		sortHubItems,
		chipMatches,
		itemMatchesQuery,
		type HubItem,
		type HubChip
	} from '#lib/components/hub/hub.js';
	import type { DMConversation, PublicUser } from '#lib/types/index.js';
	import type { PageData } from './$types';
	import { page } from '$app/state';

	let { data }: { data: PageData } = $props();

	let conversations = $state<DMConversation[]>(untrack(() => data.conversations));
	let convsCursor = $state<string | null>(untrack(() => data.conversationsCursor));
	let requests = $state(untrack(() => data.requests));
	let loadingMoreConvs = $state(false);

	// Stream filters — kind chips only. ?filter=unread stays supported for
	// deep links (forwarded to the DM list server-side).
	let chip = $state<HubChip>('all');
	const unreadOnly = $derived(data.query.filter === 'unread');

	// Details rail — desktop third column (open by default); mobile drawer
	// modal opens only on an explicit toggle.
	let detailsOpen = $state(true);
	let mobileDetailsOpen = $state(false);
	let isNarrow = $state(false);
	// The rail re-opens on every new selection (mockup: always visible for
	// an open conversation) — closing is a per-view action. Booking panes
	// have no toggle of their own, so without this a user who closed the
	// rail on a DM could never reopen it for a booking.
	let lastSelKey = '';
	$effect(() => {
		const key = data.sel ? `${data.sel.kind}:${data.sel.id}` : '';
		if (key !== lastSelKey) {
			lastSelKey = key;
			if (key) detailsOpen = true;
			mobileDetailsOpen = false;
		}
	});
	onMount(() => {
		const mq = matchMedia('(max-width: 1023.5px)');
		const sync = () => (isNarrow = mq.matches);
		sync();
		mq.addEventListener('change', sync);
		return () => mq.removeEventListener('change', sync);
	});
	function toggleDetails() {
		if (isNarrow) mobileDetailsOpen = !mobileDetailsOpen;
		else detailsOpen = !detailsOpen;
	}

	// Conversation search input — debounced sync to ?q=.
	let searchValue = $state(untrack(() => data.query.q));
	let searchTimer: ReturnType<typeof setTimeout> | undefined;

	// New conversation dialog — recipient picked via Autocomplete over /users.
	let showNewConv = $state(false);
	let newRecipient = $state<AutocompleteItem | null>(null);
	let newMessage = $state('');
	let creatingConv = $state(false);
	let newConvError = $state('');

	// DM privacy settings — opened via the gear; options via AutocompleteSelect.
	let dmPrivacy = $state('everyone');
	let showPrivacy = $state(false);
	let savingPrivacy = $state(false);
	let privacyError = $state('');

	const title = $derived(t('hub.title', 'Messages — {site}'));

	const sortOptions = [
		{ value: 'recent', label: t('messages.sort_recent', 'Most recent') },
		{ value: 'oldest', label: t('messages.sort_oldest', 'Oldest') },
		{ value: 'unread', label: t('messages.sort_unread', 'Unread first') },
		{ value: 'name', label: t('messages.sort_name', 'Name (A–Z)') }
	];
	const pageSizeOptions = [
		{ value: '10', label: '10' },
		{ value: '20', label: '20' },
		{ value: '30', label: '30' },
		{ value: '50', label: '50' }
	];
	const privacyOptions = [
		{ value: 'everyone', label: t('messages.privacy_everyone', 'Everyone') },
		{ value: 'followers_only', label: t('messages.privacy_followers', 'Followers only') },
		{ value: 'nobody', label: t('messages.privacy_nobody', 'Nobody') }
	];

	const chips: { value: HubChip; label: string }[] = $derived([
		{ value: 'all', label: t('hub.chip_all', 'All') },
		{ value: 'direct', label: t('hub.chip_direct', 'Direct') },
		{ value: 'services', label: t('hub.chip_services', 'Services') },
		{ value: 'mentorship', label: t('hub.chip_mentorship', 'Mentorship') },
		{ value: 'support', label: t('hub.chip_support', 'Support') },
		{ value: 'ai', label: t('hub.chip_ai', 'AI') }
	]);

	// The unified stream — normalize every source into HubItems, then
	// apply chip/search/state filters and sort.
	const streamItems = $derived.by(() => {
		const items: HubItem[] = [
			...conversations.map((c) => dmToHubItem(c)),
			...requests.map((r) => requestToHubItem(r)),
			...data.bookingsSent.map((b) => bookingToHubItem(b, 'sent')),
			...data.bookingsReceived.map((b) => bookingToHubItem(b, 'received')),
			...data.tickets.map((tk) => ticketToHubItem(tk, t)),
			...data.aiConversations.map((c) => aiToHubItem(c, t))
		];
		const q = data.query.q;
		// filter=unread is forwarded to the DM list server-side — apply the
		// same predicate to the non-DM sources here.
		return items.filter(
			(i) =>
				chipMatches(i, chip) && itemMatchesQuery(i, q) && (!unreadOnly || i.unread > 0)
		);
	});

	const hubItems = $derived(sortHubItems(streamItems, data.query.sort));

	// Per-kind "View all" — a chip-filtered stream only holds the newest
	// SECONDARY_PAGE_SIZE rows per source; the dedicated surfaces carry
	// the full history. Direct/requests have no separate "all" surface —
	// the hub IS the messages surface.
	const viewAll = $derived.by(() => {
		const map = {
			services: { href: '/messages/bookings', key: 'hub.view_all_services' },
			mentorship: { href: '/messages/mentorship', key: 'hub.view_all_mentorship' },
			support: { href: '/messages/support', key: 'hub.view_all_support' },
			ai: { href: '/ai', key: 'hub.ai_open_full' }
		} as Partial<Record<HubChip, { href: string; key: string }>>;
		return map[chip] ?? null;
	});

	const activeSel = $derived(data.sel);
	const activeItem = $derived(
		hubItems.find((i) => {
			if (!activeSel) return false;
			if (activeSel.kind === 'dm' || activeSel.kind === 'org') return i.sel === `dm:${activeSel.id}`;
			return i.sel === `${activeSel.kind}:${activeSel.id}`;
		}) ?? null
	);

	// The DM conversation for the selected thread — from the loaded list;
	// a sel= deep link outside the page falls back to a shell so the thread
	// still renders (there is no GET /dm/conversations/{id} endpoint).
	const activeDmConversation = $derived.by<DMConversation | null>(() => {
		const sel = data.sel;
		if (!sel || (sel.kind !== 'dm' && sel.kind !== 'org')) return null;
		return (
			conversations.find((c) => c.id === sel.id) ?? {
				id: sel.id,
				other_user_id: '',
				other_username: '',
				other_display_name: '',
				other_avatar_url: '',
				last_message_at: null,
				last_message_preview: null,
				unread_count: 0,
				created_at: '',
				conversation_type: sel.kind === 'org' ? 'org' : 'dm'
			}
		);
	});

	// Re-sync stream sources when load() re-runs (q/sort/filter changes).
	$effect(() => {
		conversations = data.conversations;
		convsCursor = data.conversationsCursor;
		requests = data.requests;
		searchValue = data.query.q;
	});

	/** Build a /messages href preserving list params; pass sel: null to clear. */
	function listHref(
		overrides: {
			q?: string;
			filter?: string | null;
			sort?: string;
			pageSize?: number;
			sel?: { kind: string; id: string } | null;
		} = {}
	): string {
		const params = new URLSearchParams();
		const q = overrides.q ?? data.query.q;
		const filter = overrides.filter === undefined ? data.query.filter : overrides.filter;
		const sort = overrides.sort ?? data.query.sort;
		const pageSize = overrides.pageSize ?? data.query.pageSize;
		const sel = overrides.sel === undefined ? data.sel : overrides.sel;
		if (q) params.set('q', q);
		if (filter) params.set('filter', filter);
		if (sort && sort !== 'recent') params.set('sort', sort);
		if (pageSize && pageSize !== 30) params.set('page_size', String(pageSize));
		if (sel) params.set('sel', `${sel.kind}:${sel.id}`);
		const qs = params.toString();
		return `/messages${qs ? `?${qs}` : ''}`;
	}

	function onSearchInput(value: string) {
		searchValue = value;
		clearTimeout(searchTimer);
		searchTimer = setTimeout(() => {
			goto(listHref({ q: value.trim() }), { reset: false });
		}, 300);
	}

	function selectItem(item: HubItem) {
		goto(listHref({ sel: { kind: item.kind, id: itemSelId(item) } }), { reset: false });
	}

	function itemSelId(item: HubItem): string {
		switch (item.kind) {
			case 'dm':
			case 'org':
				return item.dm!.id;
			case 'request':
				return item.request!.id;
			case 'booking':
			case 'mentorship':
				return item.booking!.id;
			case 'ticket':
				return item.ticket!.id;
			case 'ai':
				return item.ai!.conversation_id;
		}
	}

	function closeItem() {
		goto(listHref({ sel: null }), { reset: false });
	}

	function convQueryString(): string {
		const params = new URLSearchParams();
		params.set('page_size', String(data.query.pageSize));
		if (data.query.q) params.set('q', data.query.q);
		if (data.query.filter) params.set('filter', data.query.filter);
		if (data.query.sort !== 'recent') params.set('sort', data.query.sort);
		return params.toString();
	}

	async function loadMoreConversations() {
		if (loadingMoreConvs || !convsCursor) return;
		loadingMoreConvs = true;
		const res = await api.getEnvelope<DMConversation[]>(
			`/dm/conversations?${convQueryString()}&cursor=${encodeURIComponent(convsCursor)}`
		);
		loadingMoreConvs = false;
		const items = res.data ?? [];
		convsCursor = (res.envelope?.next_cursor as string | null | undefined) ?? null;
		if (items.length > 0) {
			const seen = new Set(conversations.map((c) => c.id));
			conversations = [...conversations, ...items.filter((c) => !seen.has(c.id))];
		}
	}

	// Live DM delivery — bump unread/preview on list rows; the open thread
	// appends its own bubbles inside DmThreadPane.
	onMount(() => {
		const handler = (ev: DMMessageEvent) => {
			if (ev.sender_id === page.data.user?.id) return;
			if (ev.conversation_id === data.sel?.id) return;
			const known = conversations.some((c) => c.id === ev.conversation_id);
			if (known) {
				conversations = conversations.map((c) =>
					c.id === ev.conversation_id
						? {
								...c,
								unread_count: c.unread_count + 1,
								last_message_preview: ev.body,
								last_message_at: new Date().toISOString()
							}
						: c
				);
			} else {
				void api
					.get<DMConversation[]>(`/dm/conversations?${convQueryString()}`)
					.then((res) => {
						if (res.data) conversations = res.data;
					});
			}
		};
		realtime.onDMMessage(handler);
		return () => realtime.offDMMessage(handler);
	});

	// Load DM privacy setting on mount.
	onMount(async () => {
		const res = await api.get<{ privacy: string }>('/dm/privacy');
		if (res.data) dmPrivacy = res.data.privacy;
	});

	async function savePrivacy(value: string) {
		privacyError = '';
		savingPrivacy = true;
		const res = await api.put('/dm/privacy', { privacy: value });
		savingPrivacy = false;
		if (res.error) {
			privacyError = t('messages.privacy_error', 'Could not save your DM privacy setting.');
			return;
		}
		dmPrivacy = value;
		showPrivacy = false;
	}

	async function fetchRecipientUsers(q: string): Promise<AutocompleteItem[]> {
		const res = await api.get<PublicUser[]>(
			`/users?q=${encodeURIComponent(q)}&page_size=10&sort=followers`
		);
		return (res.data ?? [])
			.filter((u) => u.id !== page.data.user?.id)
			.map((u) => ({
				id: u.id,
				label: u.display_name || u.username,
				sublabel: `@${u.username}`
			}));
	}

	function openNewConv() {
		newRecipient = null;
		newMessage = '';
		newConvError = '';
		showNewConv = true;
	}

	async function startNewConversation() {
		if (!newRecipient || !newMessage.trim() || creatingConv) return;
		creatingConv = true;
		newConvError = '';
		const res = await api.post<{ conversation_id: string }>('/dm/conversations', {
			recipient_id: newRecipient.id,
			message: newMessage.trim()
		});
		creatingConv = false;
		if (res.error) {
			newConvError =
				res.error.message ?? t('messages.new_conv_error', 'Could not start the conversation.');
			return;
		}
		if (res.data) {
			const convId = res.data.conversation_id;
			showNewConv = false;
			newRecipient = null;
			newMessage = '';
			await goto(listHref({ sel: { kind: 'dm', id: convId } }));
		}
	}

</script>

<SeoHead title={title} description={title} ogTitle={title} robots="noindex, nofollow" />

<!-- Exact-height column so the panes below scroll internally instead of
     growing the document. Below 768px the shell renders a sticky top bar
     (~3.5rem) and a fixed bottom tab bar (~5rem) outside this wrapper, so
     those (plus safe-area insets) are subtracted from the viewport height;
     at md+ the wrapper is the full viewport and the flex body gets whatever
     remains after the ListPageBar. -->
<div
	class="mx-auto flex w-full max-w-full flex-col overflow-hidden h-[calc(100dvh-8.5rem-env(safe-area-inset-top)-env(safe-area-inset-bottom))] md:h-dvh"
	style="--page-gutter: 1rem"
>
	<ListPageBar>
		<MessagesTabs value="inbox" />
		{#snippet actions()}
			{#if requests.length > 0}
				<!-- Pending DM requests — the stream shows them inline, this
				     badge links to the full management page for depth. -->
				<a
					href="/messages/requests"
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-text)] transition-colors hover:bg-[var(--color-primary)]/10"
				>
					{t('messages.tab_requests', 'Requests')} ({requests.length}{requests.length >= 50 ? '+' : ''})
				</a>
			{/if}
			<button
				type="button"
				onclick={openNewConv}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
				aria-label={t('messages.new_conversation', 'New conversation')}
			>
				<Plus size={14} />
				{t('hub.new_message', 'New message')}
			</button>
			<button
				type="button"
				onclick={() => (showPrivacy = true)}
				class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
				aria-label={t('messages.privacy', 'DM privacy')}
			>
				<Settings size={18} />
			</button>
		{/snippet}
	</ListPageBar>

	<div class="flex min-h-0 flex-1" style="margin-inline: var(--page-gutter)">
		<!-- Stream pane — unified conversation list. -->
		<aside
			class="flex w-full shrink-0 flex-col border-e border-[var(--color-border)] sm:max-w-xs lg:max-w-sm sm:flex"
			class:hidden={!!activeSel}
		>
			<div class="space-y-2 border-b border-[var(--color-border)] px-3 py-3">
				<!-- Search + sort share one row — search keeps the flex width,
				     the sort dropdown shrinks to its label. -->
				<div class="flex items-center gap-2">
					<div class="relative min-w-0 flex-1">
						<Search
							size={15}
							class="pointer-events-none absolute start-3 top-1/2 -translate-y-1/2 text-[var(--color-muted)]"
						/>
						<input
							type="search"
							value={searchValue}
							oninput={(e) => onSearchInput(e.currentTarget.value)}
							placeholder={t('hub.search_placeholder', 'Search')}
							aria-label={t('hub.search_placeholder', 'Search')}
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-1.5 ps-9 pe-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
						/>
					</div>
					<AutocompleteSelect
						class="w-36 shrink-0"
						options={sortOptions}
						value={data.query.sort}
						onValueChange={(v) => goto(listHref({ sort: v }), { reset: false })}
						ariaLabel={t('messages.sort_label', 'Sort conversations')}
					/>
				</div>
				<!-- One filter row — kind chips only. -->
				<div class="flex flex-wrap items-center gap-1.5">
					{#each chips as c (c.value)}
						<button
							type="button"
							onclick={() => (chip = c.value)}
							class="rounded-full px-2.5 py-1 text-[11px] font-medium transition-colors {chip ===
							c.value
								? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
								: 'bg-[var(--color-surface-subtle)] text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
						>
							{c.label}
						</button>
					{/each}
				</div>
			</div>

			{#if data.loadError}
				<div class="mx-auto w-full max-w-3xl p-4">
					<EmptyState
						icon={MessageSquareOff}
						title={t('messages.load_error', 'Could not load conversations')}
					/>
				</div>
			{:else if hubItems.length === 0}
				<div class="p-4">
					<EmptyState
						icon={MessageSquareOff}
						title={t('hub.no_items', 'Nothing here yet')}
					/>
				</div>
			{:else}
				<ul class="min-h-0 flex-1 overflow-y-auto">
					{#each hubItems as item (item.sel)}
						<HubListItem {item} active={item.sel === activeItem?.sel} onSelect={selectItem} />
					{/each}
					{#if viewAll}
						<li class="border-t border-[var(--color-border)]">
							<a
								href={viewAll.href}
								class="flex items-center justify-center gap-1.5 px-3 py-3 text-xs font-medium text-[var(--color-primary-text)] hover:bg-[var(--color-surface-subtle)]"
							>
								{t(viewAll.key, 'View all')}
								<ExternalLink size={12} aria-hidden="true" />
							</a>
						</li>
					{/if}
				</ul>

				<div
					class="flex items-center justify-between gap-2 border-t border-[var(--color-border)] px-3 py-2"
				>
					{#if convsCursor}
						<button
							type="button"
							onclick={loadMoreConversations}
							disabled={loadingMoreConvs}
							class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
						>
							{#if loadingMoreConvs}<Loader2 size={12} class="animate-spin" />{/if}
							{t('messages.load_more', 'Load more')}
						</button>
					{:else}
						<span></span>
					{/if}
					<div class="w-20">
						<AutocompleteSelect
							options={pageSizeOptions}
							value={String(data.query.pageSize)}
							onValueChange={(v) => goto(listHref({ pageSize: Number(v) }), { reset: false })}
							ariaLabel={t('messages.page_size', 'Conversations per page')}
							openUp
						/>
					</div>
				</div>
			{/if}
		</aside>

		<!-- Thread pane -->
		<section
			class="min-w-0 flex-1 flex-col sm:flex"
			class:flex={!!activeSel}
			class:hidden={!activeSel}
		>
			{#if !activeSel}
				<div class="hidden flex-1 place-items-center p-8 sm:grid">
					<EmptyState
						icon={MessageSquareOff}
						title={t('hub.select_item', 'Select a conversation')}
					/>
				</div>
			{:else if activeSel.kind === 'dm' || activeSel.kind === 'org'}
				{#if activeDmConversation}
					<DmThreadPane
						conversation={activeDmConversation}
						initialMessages={data.activeMessages}
						initialCursor={data.messagesCursor}
						pageSize={data.messagesPageSize}
						onBack={closeItem}
						onToggleDetails={toggleDetails}
						onRead={(cid) => {
							const c = conversations.find((x) => x.id === cid);
							if (c) c.unread_count = 0;
						}}
					/>
				{:else}
					<div class="grid flex-1 place-items-center p-8">
						<EmptyState
							icon={MessageSquareOff}
							title={t('messages.load_error', 'Could not load conversations')}
						/>
					</div>
				{/if}
			{:else if activeSel.kind === 'request'}
				{@const req = requests.find((r) => r.id === activeSel.id)}
				{#if req}
					<RequestPane
						request={req}
						onBack={closeItem}
						onToggleDetails={toggleDetails}
						onResolved={(id) => (requests = requests.filter((r) => r.id !== id))}
						onAccepted={(cid) => goto(listHref({ sel: { kind: 'dm', id: cid } }))}
					/>
				{:else}
					<div class="grid flex-1 place-items-center p-8">
						<EmptyState icon={MessageSquareOff} title={t('hub.item_missing', 'Not found')} />
					</div>
				{/if}
			{:else if activeSel.kind === 'booking' || activeSel.kind === 'mentorship'}
				{@const booking =
					data.selectedBooking ??
					[...data.bookingsSent, ...data.bookingsReceived].find(
						(b) => b.id === activeSel.id
					)}
				{#if booking}
					<BookingDetailPane
						{booking}
						role={data.bookingsReceived.some((b) => b.id === booking.id)
							? 'received'
							: 'sent'}
						viewerId={page.data.viewerId ?? ''}
						viewerEmail={page.data.user?.email ?? ''}
						dmEnabled
						packagesEnabled={!!page.data.features?.['service_packages.enabled']}
						onClose={closeItem}
					/>
				{:else}
					<div class="grid flex-1 place-items-center p-8">
						<EmptyState icon={MessageSquareOff} title={t('hub.item_missing', 'Not found')} />
					</div>
				{/if}
			{:else if activeSel.kind === 'ticket'}
				{#if data.ticketDetail}
					<TicketThreadPane
						detail={data.ticketDetail}
						onBack={closeItem}
						onToggleDetails={toggleDetails}
					/>
				{:else}
					<div class="grid flex-1 place-items-center p-8">
						<EmptyState icon={MessageSquareOff} title={t('hub.item_missing', 'Not found')} />
					</div>
				{/if}
			{:else if activeSel.kind === 'ai'}
				<AiThreadPane
					conversationId={activeSel.id}
					onBack={closeItem}
					onToggleDetails={toggleDetails}
				/>
			{/if}
		</section>

		<!-- Details rail — third column on lg+, part of the reference layout. -->
		{#if activeItem && detailsOpen}
			<div class="hidden w-72 shrink-0 border-s border-[var(--color-border)] lg:block">
				<ConversationDetailsPanel item={activeItem} ticketDetail={data.ticketDetail} />
			</div>
		{/if}
	</div>
</div>

<!-- Details drawer for smaller viewports — the lg+ rail is inline above. -->
{#if isNarrow && activeItem}
	<Modal
		bind:open={mobileDetailsOpen}
		title={t('hub.details_toggle', 'Conversation details')}
		size="md"
		zIndex={50}
	>
		<ConversationDetailsPanel item={activeItem} ticketDetail={data.ticketDetail} />
	</Modal>
{/if}

<!-- New conversation dialog — recipient via user-search Autocomplete. -->
<Modal
	bind:open={showNewConv}
	title={t('messages.new_conversation', 'New conversation')}
	size="md"
	zIndex={60}
>
	<form
		onsubmit={(e) => {
			e.preventDefault();
			startNewConversation();
		}}
		class="space-y-3"
	>
		<div>
			<label
				for="dm-recipient"
				class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]"
				>{t('messages.recipient', 'To')}</label
			>
			<Autocomplete
				placeholder={t('messages.recipient_placeholder', 'Search people by name or @username')}
				ariaLabel={t('messages.recipient', 'To')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={fetchRecipientUsers}
				selectedId={newRecipient?.id ?? null}
				selectedLabel={newRecipient?.label ?? null}
				onSelect={(item) => (newRecipient = item)}
				onClear={() => (newRecipient = null)}
			/>
		</div>
		<LiteralSuggest
			textarea
			bind:value={newMessage}
			placeholder={t('messages.placeholder', 'Type a message…')}
			rows={3}
			wrapperClass="relative w-full"
			class="w-full resize-none rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
		/>
		{#if newConvError}
			<p class="text-sm text-[var(--color-danger)]">{newConvError}</p>
		{/if}
		<div class="flex justify-end gap-2">
			<button
				type="button"
				onclick={() => (showNewConv = false)}
				class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
			>
				{t('common.cancel', 'Cancel')}
			</button>
			<button
				type="submit"
				disabled={creatingConv || !newRecipient || !newMessage.trim()}
				class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
			>
				{t('messages.send', 'Send')}
			</button>
		</div>
	</form>
</Modal>

<!-- DM privacy dialog — options via AutocompleteSelect. -->
<Modal
	bind:open={showPrivacy}
	title={t('messages.privacy', 'DM privacy')}
	description={t('messages.privacy_desc', 'Who can start a direct message with you.')}
	size="sm"
	zIndex={60}
>
	<div class="space-y-3">
		<AutocompleteSelect
			options={privacyOptions}
			value={dmPrivacy}
			onValueChange={(v) => savePrivacy(v)}
			ariaLabel={t('messages.privacy', 'DM privacy')}
			disabled={savingPrivacy}
		/>
		{#if privacyError}
			<p class="text-sm text-[var(--color-danger)]">{privacyError}</p>
		{/if}
	</div>
</Modal>

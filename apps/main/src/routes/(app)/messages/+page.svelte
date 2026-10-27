<script lang="ts">
	import { untrack, onMount, tick } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import {
		Send,
		MessageSquareOff,
		ArrowLeft,
		Pencil,
		Trash2,
		Plus,
		Settings,
		Search,
		Loader2,
		Flag,
		Package
	} from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import {
		EmptyState,
		LiteralSuggest,
		Modal,
		AutocompleteSelect,
		ListPageBar
	} from '$lib/components/ui';
	import MessagesTabs from '$lib/components/messages/MessagesTabs.svelte';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { realtime, type DMMessageEvent } from '$lib/realtime/realtime.svelte';
	import ReportDialog from '$lib/components/profile/ReportDialog.svelte';
	import type { DMConversation, DMMessage, PublicUser } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let conversations = $state<DMConversation[]>(untrack(() => data.conversations));
	let convsCursor = $state<string | null>(untrack(() => data.conversationsCursor));
	let messages = $state<DMMessage[]>(untrack(() => data.activeMessages));
	let msgsCursor = $state<string | null>(untrack(() => data.messagesCursor));
	let loadingMoreConvs = $state(false);
	let loadingEarlier = $state(false);
	let draft = $state('');
	let sending = $state(false);

	// Edit/delete message state (15-min window enforced server-side).
	let editingId = $state<string | null>(null);
	let editBody = $state('');
	let deletingId = $state<string | null>(null);

	// Report incoming message (Section 51 via the DM report route).
	let reportingMsgId = $state<string | null>(null);
	let reportDialogOpen = $state(false);

	// DM privacy settings — opened via the gear; options via AutocompleteSelect.
	let dmPrivacy = $state('everyone');
	let showPrivacy = $state(false);
	let savingPrivacy = $state(false);
	let privacyError = $state('');

	// New conversation dialog — recipient picked via Autocomplete over /users.
	let showNewConv = $state(false);
	let newRecipient = $state<AutocompleteItem | null>(null);
	let newMessage = $state('');
	let creatingConv = $state(false);
	let newConvError = $state('');

	// Conversation search input — debounced sync to ?q=.
	let searchValue = $state(untrack(() => data.query.q));
	let searchTimer: ReturnType<typeof setTimeout> | undefined;

	let threadEl = $state<HTMLElement | null>(null);
	let lastReadMarked = $state<string | null>(null);

	const activeId = $derived(data.activeId);

	$effect(() => {
		conversations = data.conversations;
		convsCursor = data.conversationsCursor;
		messages = data.activeMessages;
		msgsCursor = data.messagesCursor;
		searchValue = data.query.q;
	});

	const activeConversation = $derived(
		conversations.find((c) => c.id === activeId) ?? null
	);

	const title = $derived(t('messages.title', 'Messages — {site}'));

	const sortOptions = [
		{ value: 'recent', label: t('messages.sort_recent', 'Most recent') },
		{ value: 'oldest', label: t('messages.sort_oldest', 'Oldest') },
		{ value: 'unread', label: t('messages.sort_unread', 'Unread first') },
		{ value: 'name', label: t('messages.sort_name', 'Name (A–Z)') }
	];
	const filterOptions = [
		{ value: '', label: t('messages.filter_all', 'All conversations') },
		{ value: 'unread', label: t('messages.filter_unread', 'Unread only') }
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

	/** Build a /messages href preserving list params; pass c: null to clear. */
	function listHref(overrides: {
		q?: string;
		filter?: string | null;
		sort?: string;
		pageSize?: number;
		c?: string | null;
	} = {}): string {
		const params = new URLSearchParams();
		const q = overrides.q ?? data.query.q;
		const filter = overrides.filter === undefined ? data.query.filter : overrides.filter;
		const sort = overrides.sort ?? data.query.sort;
		const pageSize = overrides.pageSize ?? data.query.pageSize;
		const c = overrides.c === undefined ? activeId : overrides.c;
		if (q) params.set('q', q);
		if (filter) params.set('filter', filter);
		if (sort && sort !== 'recent') params.set('sort', sort);
		if (pageSize && pageSize !== 30) params.set('page_size', String(pageSize));
		if (c) params.set('c', c);
		const qs = params.toString();
		return `/messages${qs ? `?${qs}` : ''}`;
	}

	function onSearchInput(value: string) {
		searchValue = value;
		clearTimeout(searchTimer);
		searchTimer = setTimeout(() => {
			goto(listHref({ q: value.trim() }), { keepFocus: true, noScroll: true });
		}, 300);
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

	async function loadEarlierMessages() {
		if (loadingEarlier || !msgsCursor || !activeId) return;
		loadingEarlier = true;
		const res = await api.getEnvelope<DMMessage[]>(
			`/dm/conversations/${activeId}/messages?page_size=${data.messagesPageSize}&cursor=${encodeURIComponent(msgsCursor)}`
		);
		loadingEarlier = false;
		const items = res.data ?? [];
		msgsCursor = (res.envelope?.next_cursor as string | null | undefined) ?? null;
		// API is newest-first — reverse to chronological and prepend.
		messages = [...items.reverse(), ...messages];
	}

	function selectConversation(id: string) {
		goto(listHref({ c: id }), { noScroll: true });
	}

	function closeConversation() {
		goto(listHref({ c: null }), { noScroll: true });
	}

	// Mark the active conversation read once the server payload has it.
	$effect(() => {
		const id = data.activeId;
		if (!id || lastReadMarked === id) return;
		lastReadMarked = id;
		void api.post(`/dm/conversations/${id}/read`).then((res) => {
			if (!res.error) {
				conversations = conversations.map((c) =>
					c.id === id ? { ...c, unread_count: 0 } : c
				);
			}
		});
	});

	// Scroll the thread to the newest message when the conversation changes.
	$effect(() => {
		if (activeId && threadEl) {
			tick().then(() => {
				if (threadEl) threadEl.scrollTop = threadEl.scrollHeight;
			});
		}
	});

	// Load DM privacy setting on mount.
	onMount(async () => {
		const res = await api.get<{ privacy: string }>('/dm/privacy');
		if (res.data) dmPrivacy = res.data.privacy;
	});

	// Live DM delivery via WebSocket. Skip events from the current user —
	// the optimistic append in sendMessage already added the bubble.
	onMount(() => {
		const handler = (ev: DMMessageEvent) => {
			if (ev.sender_id === page.data.user?.id) return;

			if (ev.conversation_id === activeId) {
				messages = [
					...messages,
					{
						id: crypto.randomUUID(),
						sender_id: ev.sender_id,
						sender_username: '',
						sender_display_name: ev.sender_display_name ?? '',
						body: ev.body,
						created_at: new Date().toISOString(),
						read_at: null,
						context_type: ev.context_type,
						context_id: ev.context_id,
						context_label: ev.context_label,
						context_slug: ev.context_slug,
						sent_as_org: ev.sent_as_org
					}
				];
				// Keep the server-side unread count accurate for the open thread.
				void api.post(`/dm/conversations/${ev.conversation_id}/read`);
			} else {
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
					// New conversation we haven't loaded — refetch the first page.
					void api
						.get<DMConversation[]>(`/dm/conversations?${convQueryString()}`)
						.then((res) => {
							if (res.data) conversations = res.data;
						});
				}
			}
		};
		realtime.onDMMessage(handler);
		return () => realtime.offDMMessage(handler);
	});

	async function sendMessage() {
		if (!activeId || !draft.trim() || sending) return;
		const body = draft.trim();
		draft = '';
		sending = true;
		const tempId = crypto.randomUUID();
		messages = [
			...messages,
			{
				id: tempId,
				sender_id: page.data.user?.id ?? '',
				sender_username: page.data.user?.username ?? '',
				sender_display_name: page.data.user?.display_name ?? '',
				body,
				created_at: new Date().toISOString(),
				read_at: null
			}
		];
		tick().then(() => {
			if (threadEl) threadEl.scrollTop = threadEl.scrollHeight;
		});
		const res = await api.post<{ status: string }>(`/dm/conversations/${activeId}/messages`, {
			body
		});
		sending = false;
		if (res.error) {
			messages = messages.filter((m) => m.id !== tempId);
			draft = body;
		}
	}

	async function startEdit(msg: DMMessage) {
		editingId = msg.id;
		editBody = msg.body;
		// Fetch the raw (unresolved) body so hand-typed {{...}} literals survive
		// a re-save — the display payload has them already resolved.
		if (!activeId) return;
		const res = await api.get<DMMessage[]>(
			`/dm/conversations/${activeId}/messages?raw=1&page_size=100`
		);
		const raw = res.data?.find((m) => m.id === msg.id);
		if (editingId === msg.id && raw) editBody = raw.body;
	}

	async function saveEdit(msg: DMMessage) {
		if (!activeId || !editBody.trim() || editingId !== msg.id) return;
		const res = await api.patch(`/dm/conversations/${activeId}/messages/${msg.id}`, {
			body: editBody.trim()
		});
		if (!res.error) {
			messages = messages.map((m) =>
				m.id === msg.id ? { ...m, body: editBody.trim() } : m
			);
		}
		editingId = null;
		editBody = '';
	}

	async function deleteMessage(msg: DMMessage) {
		if (!activeId || deletingId) return;
		deletingId = msg.id;
		const res = await api.delete(`/dm/conversations/${activeId}/messages/${msg.id}`);
		deletingId = null;
		if (!res.error) {
			messages = messages.filter((m) => m.id !== msg.id);
		}
	}

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
			const sentBody = newMessage.trim();
			showNewConv = false;
			newRecipient = null;
			newMessage = '';
			if (convId === activeId) {
				// Navigating to the same ?c= is a no-op — append the first
				// message locally so it isn't lost until the next load.
				messages = [
					...messages,
					{
						id: crypto.randomUUID(),
						sender_id: page.data.user?.id ?? '',
						sender_username: page.data.user?.username ?? '',
						sender_display_name: page.data.user?.display_name ?? '',
						body: sentBody,
						created_at: new Date().toISOString(),
						read_at: null
					}
				];
			} else {
				await goto(listHref({ c: convId }));
			}
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

<!-- Exact-height column so the panes below scroll internally instead of
     growing the document. Below 768px the shell renders a sticky top bar
     (~3.5rem) and a fixed bottom tab bar (~5rem) outside this wrapper, so
     those (plus safe-area insets) are subtracted from the viewport height;
     at md+ the wrapper is the full viewport and the flex body gets whatever
     remains after the ListPageBar. -->
<div
	class="mx-auto flex w-full max-w-full flex-col overflow-hidden h-[calc(100dvh-8.5rem-env(safe-area-inset-top)-env(safe-area-inset-bottom))] md:h-dvh"
>
	<ListPageBar>
		<MessagesTabs value="inbox" requestCount={data.requestCount} />
		{#snippet actions()}
			<button
				type="button"
				onclick={openNewConv}
				class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
				aria-label={t('messages.new_conversation', 'New conversation')}
			>
				<Plus size={18} />
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

	<div class="flex min-h-0 flex-1 gap-0">
		<!-- Conversation list -->
		<aside
			class="flex w-full shrink-0 flex-col border-e border-[var(--color-border)] sm:max-w-xs sm:flex"
			class:hidden={!!activeId}
		>
			<div class="space-y-2 border-b border-[var(--color-border)] px-3 py-3">
				<div class="relative">
					<Search
						size={15}
						class="pointer-events-none absolute start-3 top-1/2 -translate-y-1/2 text-[var(--color-muted)]"
					/>
					<input
						type="search"
						value={searchValue}
						oninput={(e) => onSearchInput(e.currentTarget.value)}
						placeholder={t('messages.search_placeholder', 'Search conversations')}
						aria-label={t('messages.search_placeholder', 'Search conversations')}
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-1.5 ps-9 pe-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
					/>
				</div>
				<div class="flex gap-2">
					<div class="min-w-0 flex-1">
						<AutocompleteSelect
							options={sortOptions}
							value={data.query.sort}
							onValueChange={(v) => goto(listHref({ sort: v }), { noScroll: true })}
							ariaLabel={t('messages.sort_label', 'Sort conversations')}
						/>
					</div>
					<div class="min-w-0 flex-1">
						<AutocompleteSelect
							options={filterOptions}
							value={data.query.filter ?? ''}
							onValueChange={(v) => goto(listHref({ filter: v || null }), { noScroll: true })}
							ariaLabel={t('messages.filter_label', 'Filter conversations')}
						/>
					</div>
				</div>
			</div>

			{#if data.loadError}
				<div class="mx-auto w-full max-w-3xl p-4">
					<EmptyState
						icon={MessageSquareOff}
						title={t('messages.load_error', 'Could not load conversations')}
					/>
				</div>
			{:else if conversations.length === 0}
				<div class="p-4">
					<EmptyState
						icon={MessageSquareOff}
						title={t('messages.no_conversations', 'No conversations yet')}
					/>
				</div>
			{:else}
				<ul class="min-h-0 flex-1 overflow-y-auto">
					{#each conversations as conv (conv.id)}
						{@const active = conv.id === activeId}
						<li>
							<button
								type="button"
								onclick={() => selectConversation(conv.id)}
								aria-current={active ? 'true' : undefined}
								class="flex w-full items-center gap-3 border-b border-[var(--color-border)] px-4 py-3 text-start transition-colors {active
									? 'bg-[var(--color-surface-subtle)]'
									: 'hover:bg-[var(--color-surface-subtle)]'}"
							>
								{#if conv.other_avatar_url}
									<img
										src={conv.other_avatar_url}
										alt=""
										class="h-10 w-10 shrink-0 rounded-full object-cover"
									/>
								{:else}
									<span
										class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]"
										>{conv.other_display_name.charAt(0).toUpperCase()}</span
									>
								{/if}
								<span class="min-w-0 flex-1">
									<span class="flex items-center justify-between gap-2">
										<span class="truncate text-sm font-medium text-[var(--color-text)]"
											>{conv.other_display_name}</span
										>
										{#if conv.unread_count > 0}
											<span
												class="shrink-0 rounded-full bg-[var(--color-primary)] px-1.5 py-0.5 text-[10px] font-medium text-[var(--color-primary-contrast)]"
												>{conv.unread_count}</span
											>
										{/if}
									</span>
									<span class="block truncate text-xs text-[var(--color-muted)]"
										>{conv.last_message_preview ?? '@' + conv.other_username}</span
									>
								</span>
							</button>
						</li>
					{/each}
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
							onValueChange={(v) => goto(listHref({ pageSize: Number(v) }), { noScroll: true })}
							ariaLabel={t('messages.page_size', 'Conversations per page')}
							openUp
						/>
					</div>
				</div>
			{/if}
		</aside>

		<!-- Message view -->
		<section class="flex min-w-0 flex-1 flex-col">
			{#if !activeId}
				<div class="grid flex-1 place-items-center p-8">
					<EmptyState
						icon={MessageSquareOff}
						title={t('messages.select_conversation', 'Select a conversation')}
					/>
				</div>
			{:else}
				<div class="flex items-center gap-3 border-b border-[var(--color-border)] px-4 py-3">
					<button
						type="button"
						onclick={closeConversation}
						class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] sm:hidden"
						aria-label={t('messages.back', 'Back')}
					>
						<ArrowLeft class="rtl:-scale-x-100" size={18} />
					</button>
					{#if activeConversation?.other_avatar_url}
						<img
							src={activeConversation.other_avatar_url}
							alt=""
							class="h-9 w-9 rounded-full object-cover"
						/>
					{:else}
						<span
							class="grid h-9 w-9 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]"
							>{(activeConversation?.other_display_name ?? '?').charAt(0).toUpperCase()}</span
						>
					{/if}
					<div class="min-w-0">
						<p class="truncate text-sm font-medium text-[var(--color-text)]">
							{activeConversation?.other_display_name}
						</p>
						{#if activeConversation?.other_username}
							<a
								href={activeConversation?.conversation_type === 'org'
									? `/org/${activeConversation.other_username}`
									: `/u/${activeConversation.other_username}`}
								class="truncate text-xs text-[var(--color-muted)] hover:text-[var(--color-primary)]"
								>@{activeConversation.other_username}</a
							>
						{/if}
					</div>
				</div>

				<div bind:this={threadEl} class="min-h-0 flex-1 overflow-y-auto p-4">
					{#if msgsCursor}
						<div class="mb-3 text-center">
							<button
								type="button"
								onclick={loadEarlierMessages}
								disabled={loadingEarlier}
								class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
							>
								{#if loadingEarlier}<Loader2 size={12} class="animate-spin" />{/if}
								{t('messages.load_earlier', 'Load earlier messages')}
							</button>
						</div>
					{/if}
					{#if messages.length === 0}
						<p class="py-8 text-center text-sm text-[var(--color-muted)]">
							{t('messages.no_messages', 'No messages yet. Say hello!')}
						</p>
					{:else}
						<ul class="space-y-2">
							{#each messages as msg (msg.id)}
								{@const mine = msg.sender_id === page.data.user?.id}
								<li class="group flex {mine ? 'justify-end' : 'justify-start'}">
									<div
										class="max-w-[75%] rounded-[var(--radius)] px-3 py-2 text-sm {mine
											? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
											: 'border border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-text)]'}"
									>
										{#if editingId === msg.id}
											<LiteralSuggest
												textarea
												bind:value={editBody}
												class="w-full resize-none rounded-[var(--radius)] bg-[var(--color-surface)] px-2 py-1 text-[var(--color-text)] focus:outline-none"
												wrapperClass="relative w-full"
												rows={2}
											/>
											<div class="mt-1 flex gap-2">
												<button
													type="button"
													onclick={() => saveEdit(msg)}
													class="text-xs font-medium underline"
												>
													{t('messages.save', 'Save')}
												</button>
												<button
													type="button"
													onclick={() => {
														editingId = null;
														editBody = '';
													}}
													class="text-xs font-medium underline"
												>
													{t('common.cancel', 'Cancel')}
												</button>
											</div>
										{:else}
											{#if msg.context_type === 'service_listing' && msg.context_slug}
												<a
													href="/services/{msg.context_slug}"
													class="mb-1.5 flex items-center gap-2 rounded-[var(--radius)] border px-2.5 py-1.5 text-xs {mine
														? 'border-[var(--color-primary-contrast)]/30 text-[var(--color-primary-contrast)]'
														: 'border-[var(--color-border)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)]'}"
												>
													<Package size={13} aria-hidden="true" />
													<span class="truncate font-medium"
														>{msg.context_label ?? msg.context_slug}</span
													>
												</a>
											{/if}
											{#if msg.sent_as_org && msg.sender_display_name}
												<p
													class="mb-0.5 text-[10px] font-medium {mine
														? 'text-[var(--color-primary-contrast)]/80'
														: 'text-[var(--color-text-subtle)]'}"
												>
													{msg.sender_display_name}
												</p>
											{/if}
											<p class="whitespace-pre-wrap break-words">{msg.body}</p>
											<p
												class="mt-1 text-[10px] {mine
													? 'text-[var(--color-primary-contrast)]/70'
													: 'text-[var(--color-muted)]'}"
											>
												{new Date(msg.created_at).toLocaleTimeString()}
											</p>
											<div
												class="mt-1 flex gap-2 opacity-0 transition-opacity group-hover:opacity-100"
											>
												{#if mine}
													<button
														type="button"
														onclick={() => startEdit(msg)}
														class="text-[10px] font-medium underline"
														aria-label={t('messages.edit', 'Edit')}
													>
														<Pencil size={12} />
													</button>
													<button
														type="button"
														onclick={() => deleteMessage(msg)}
														disabled={deletingId === msg.id}
														class="text-[10px] font-medium underline disabled:opacity-50"
														aria-label={t('messages.delete', 'Delete')}
													>
														<Trash2 size={12} />
													</button>
												{:else}
													<button
														type="button"
														onclick={() => {
															reportingMsgId = msg.id;
															reportDialogOpen = true;
														}}
														class="text-[10px] font-medium underline"
														aria-label={t('messages.report', 'Report')}
													>
														<Flag size={12} />
													</button>
												{/if}
											</div>
										{/if}
									</div>
								</li>
							{/each}
						</ul>
					{/if}
				</div>

				<form
					class="flex items-center gap-2 border-t border-[var(--color-border)] p-3"
					onsubmit={(e) => {
						e.preventDefault();
						sendMessage();
					}}
				>
					<LiteralSuggest
						bind:value={draft}
						placeholder={t('messages.placeholder', 'Type a message…')}
						class="w-full rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
					/>
					<button
						type="submit"
						disabled={sending || !draft.trim()}
						class="grid h-9 w-9 shrink-0 place-items-center rounded-full bg-[var(--color-primary)] text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
						aria-label={t('messages.send', 'Send')}
					>
						<Send class="rtl:-scale-x-100" size={16} />
					</button>
				</form>
			{/if}
		</section>
	</div>
</div>

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

<!-- Report dialog for incoming messages. -->
{#if reportingMsgId && activeId}
	<ReportDialog
		bind:open={reportDialogOpen}
		entityType="dm_message"
		entityId={reportingMsgId}
		dmConversationId={activeId}
		entityName={activeConversation?.other_display_name ?? t('messages.report', 'Report')}
	/>
{/if}

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

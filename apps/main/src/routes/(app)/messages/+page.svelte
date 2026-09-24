<script lang="ts">
	import { untrack, onMount } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { Send, MessageSquareOff, ArrowLeft, Pencil, Trash2, Plus, Settings } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { EmptyState, LiteralSuggest } from '$lib/components/ui';
	import { realtime, type DMMessageEvent } from '$lib/realtime/realtime.svelte';
	import type { DMConversation, DMMessage } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let conversations = $state<DMConversation[]>(untrack(() => data.conversations));
	let activeId = $state<string | null>(untrack(() => data.activeId));
	let messages = $state<DMMessage[]>(untrack(() => data.activeMessages));
	let draft = $state('');
	let sending = $state(false);
	let loadingMessages = $state(false);

	// BUG-021: edit/delete message state
	let editingId = $state<string | null>(null);
	let editBody = $state('');
	let deletingId = $state<string | null>(null);

	// BUG-020: DM privacy settings
	let dmPrivacy = $state('everyone');
	let showPrivacyMenu = $state(false);
	let savingPrivacy = $state(false);

	// BUG-022: new conversation dialog
	let showNewConv = $state(false);
	let newRecipientId = $state('');
	let newMessage = $state('');
	let creatingConv = $state(false);

	// Load DM privacy setting on mount
	onMount(async () => {
		const res = await api.get<{ privacy: string }>('/dm/privacy');
		if (res.data) dmPrivacy = res.data.privacy;
	});

	$effect(() => {
		conversations = data.conversations;
		activeId = data.activeId;
		messages = data.activeMessages;
	});

	const activeConversation = $derived(
		conversations.find((c) => c.id === activeId) ?? null
	);

	const title = $derived(t('messages.title', 'Messages — JapaConnect'));

	// Phase 23 — live DM delivery via WebSocket. When a dm_message event
	// arrives for the active conversation, append it; otherwise bump the
	// conversation's unread counter and last-message preview. Skip events
	// from the current user (the optimistic append in sendMessage already
	// added the bubble — the server echo would duplicate it).
	onMount(() => {
		const handler = (ev: DMMessageEvent) => {
			// BUG-007: skip server echo for messages sent by the current user
			// to prevent duplicate bubbles (optimistic UI + WS echo).
			if (ev.sender_id === page.data.user?.id) return;

			if (ev.conversation_id === activeId) {
				messages = [
					...messages,
					{
						id: crypto.randomUUID(),
						sender_id: ev.sender_id,
						sender_username: '',
						sender_display_name: '',
						body: ev.body,
						created_at: new Date().toISOString(),
						read_at: null
					}
				];
				// Best-effort: mark the conversation read so the server-side
				// unread count stays accurate for the active thread.
				void api.post(`/dm/conversations/${ev.conversation_id}/read`);
			} else {
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
			}
		};
		realtime.onDMMessage(handler);
		return () => realtime.offDMMessage(handler);
	});

	async function selectConversation(id: string) {
		activeId = id;
		loadingMessages = true;
		messages = [];
		const res = await api.get<DMMessage[]>(`/dm/conversations/${id}/messages?page_size=${data.pageSize}`);
		loadingMessages = false;
		if (res.data) messages = res.data;
		// BUG-023: clear the unread badge only after the server confirms
		// the read mark, not optimistically (Rule 16).
		const readRes = await api.post(`/dm/conversations/${id}/read`);
		if (!readRes.error) {
			conversations = conversations.map((c) =>
				c.id === id ? { ...c, unread_count: 0 } : c
			);
		}
	}

	async function sendMessage() {
		if (!activeId || !draft.trim() || sending) return;
		const body = draft.trim();
		draft = '';
		sending = true;
		// BUG-006: backend returns { data: { status: "ok" } }, not { id }.
		// Use a client-generated UUID for the optimistic bubble; the WS
		// echo is skipped for own messages (BUG-007) so no duplicate.
		const tempId = crypto.randomUUID();
		sending = false;
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
		const res = await api.post<{ status: string }>(`/dm/conversations/${activeId}/messages`, { body });
		if (res.error) {
			// Remove the optimistic bubble on failure and restore the draft.
			messages = messages.filter((m) => m.id !== tempId);
			draft = body;
		}
	}

	// BUG-021: edit own message (15-min window enforced server-side)
	async function startEdit(msg: DMMessage) {
		editingId = msg.id;
		editBody = msg.body;
		// Fetch the raw (unresolved) body so hand-typed {{...}} literals survive
		// a re-save — the display payload has them already resolved.
		if (!activeId) return;
		const res = await api.get<DMMessage[]>(`/dm/conversations/${activeId}/messages?raw=1`);
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

	// BUG-021: soft-delete own message
	async function deleteMessage(msg: DMMessage) {
		if (!activeId || deletingId) return;
		deletingId = msg.id;
		const res = await api.delete(`/dm/conversations/${activeId}/messages/${msg.id}`);
		deletingId = null;
		if (!res.error) {
			messages = messages.filter((m) => m.id !== msg.id);
		}
	}

	// BUG-020: save DM privacy setting
	async function savePrivacy(value: string) {
		savingPrivacy = true;
		const res = await api.put('/dm/privacy', { privacy: value });
		savingPrivacy = false;
		if (!res.error) {
			dmPrivacy = value;
			showPrivacyMenu = false;
		}
	}

	// BUG-022: start a new conversation
	async function startNewConversation() {
		if (!newRecipientId.trim() || !newMessage.trim() || creatingConv) return;
		creatingConv = true;
		const res = await api.post<{ conversation_id: string }>('/dm/conversations', {
			recipient_id: newRecipientId.trim(),
			message: newMessage.trim()
		});
		creatingConv = false;
		if (res.data) {
			showNewConv = false;
			newRecipientId = '';
			newMessage = '';
			await goto(`/messages?c=${res.data.conversation_id}`);
		}
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={title} />
	<meta property="og:title" content={title} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto flex w-full max-w-full gap-0">
	<!-- Conversation list -->
	<aside class="flex w-full max-w-xs shrink-0 flex-col border-r border-[var(--color-border)]" class:hidden={!!activeId && conversations.length > 0}>
		<div class="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
			<h1 class="text-base font-semibold text-[var(--color-text)]">{t('messages.heading', 'Messages')}</h1>
			<div class="flex items-center gap-1">
				<!-- BUG-022: start new conversation -->
				<button
					type="button"
					onclick={() => (showNewConv = true)}
					class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
					aria-label={t('messages.new_conversation', 'New conversation')}
				>
					<Plus size={18} />
				</button>
				<!-- BUG-020: DM privacy settings -->
				<div class="relative">
					<button
						type="button"
						onclick={() => (showPrivacyMenu = !showPrivacyMenu)}
						class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
						aria-label={t('messages.privacy', 'DM privacy')}
					>
						<Settings size={18} />
					</button>
					{#if showPrivacyMenu}
						<div class="absolute right-0 top-full z-10 mt-1 w-48 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-1 shadow-lg">
							<button
								type="button"
								onclick={() => savePrivacy('everyone')}
								disabled={savingPrivacy}
								class="flex w-full items-center justify-between px-3 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
							>
								{t('messages.privacy_everyone', 'Everyone')}
								{#if dmPrivacy === 'everyone'}<span class="text-[var(--color-primary)]">✓</span>{/if}
							</button>
							<button
								type="button"
								onclick={() => savePrivacy('followers_only')}
								disabled={savingPrivacy}
								class="flex w-full items-center justify-between px-3 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
							>
								{t('messages.privacy_followers', 'Followers only')}
								{#if dmPrivacy === 'followers_only'}<span class="text-[var(--color-primary)]">✓</span>{/if}
							</button>
							<button
								type="button"
								onclick={() => savePrivacy('nobody')}
								disabled={savingPrivacy}
								class="flex w-full items-center justify-between px-3 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
							>
								{t('messages.privacy_nobody', 'Nobody')}
								{#if dmPrivacy === 'nobody'}<span class="text-[var(--color-primary)]">✓</span>{/if}
							</button>
						</div>
					{/if}
				</div>
			</div>
		</div>
		{#if conversations.length === 0}
			<div class="p-4">
				<EmptyState icon={MessageSquareOff} title={t('messages.no_conversations', 'No conversations yet')} />
			</div>
		{:else}
			<ul class="flex-1 overflow-y-auto">
				{#each conversations as conv (conv.id)}
					{@const active = conv.id === activeId}
					<li>
						<button
							type="button"
							onclick={() => selectConversation(conv.id)}
							aria-current={active ? 'true' : undefined}
							class="flex w-full items-center gap-3 border-b border-[var(--color-border)] px-4 py-3 text-left transition-colors {active ? 'bg-[var(--color-surface-subtle)]' : 'hover:bg-[var(--color-surface-subtle)]'}"
						>
							{#if conv.other_avatar_url}
								<img src={conv.other_avatar_url} alt="" class="h-10 w-10 shrink-0 rounded-full object-cover" />
							{:else}
								<span class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]">{conv.other_display_name.charAt(0).toUpperCase()}</span>
							{/if}
							<span class="min-w-0 flex-1">
								<span class="flex items-center justify-between gap-2">
									<span class="truncate text-sm font-medium text-[var(--color-text)]">{conv.other_display_name}</span>
									{#if conv.unread_count > 0}
										<span class="shrink-0 rounded-full bg-[var(--color-primary)] px-1.5 py-0.5 text-[10px] font-medium text-[var(--color-primary-contrast)]">{conv.unread_count}</span>
									{/if}
								</span>
								<span class="block truncate text-xs text-[var(--color-muted)]">{conv.last_message_preview ?? '@' + conv.other_username}</span>
							</span>
						</button>
					</li>
				{/each}
			</ul>
		{/if}
	</aside>

	<!-- Message view -->
	<section class="flex min-w-0 flex-1 flex-col">
		{#if !activeId}
			<div class="grid flex-1 place-items-center p-8">
				<EmptyState icon={MessageSquareOff} title={t('messages.select_conversation', 'Select a conversation')} />
			</div>
		{:else}
			<div class="flex items-center gap-3 border-b border-[var(--color-border)] px-4 py-3">
				<button type="button" onclick={() => (activeId = null)} class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] md:hidden" aria-label={t('messages.back', 'Back')}>
					<ArrowLeft size={18} />
				</button>
				{#if activeConversation?.other_avatar_url}
					<img src={activeConversation.other_avatar_url} alt="" class="h-9 w-9 rounded-full object-cover" />
				{:else}
					<span class="grid h-9 w-9 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]">{(activeConversation?.other_display_name ?? '?').charAt(0).toUpperCase()}</span>
				{/if}
				<div class="min-w-0">
					<p class="truncate text-sm font-medium text-[var(--color-text)]">{activeConversation?.other_display_name}</p>
					<p class="truncate text-xs text-[var(--color-muted)]">@{activeConversation?.other_username}</p>
				</div>
			</div>

			<div class="flex-1 overflow-y-auto p-4">
				{#if loadingMessages}
					<p class="py-8 text-center text-sm text-[var(--color-muted)]">{t('common.loading')}</p>
				{:else if messages.length === 0}
					<p class="py-8 text-center text-sm text-[var(--color-muted)]">{t('messages.no_messages', 'No messages yet. Say hello!')}</p>
				{:else}
					<ul class="space-y-2">
						{#each messages as msg (msg.id)}
							{@const mine = msg.sender_id === page.data.user?.id}
							<li class="group flex {mine ? 'justify-end' : 'justify-start'}">
								<div class="max-w-[75%] rounded-[var(--radius)] px-3 py-2 text-sm {mine ? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]' : 'border border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-text)]'}">
									{#if editingId === msg.id}
										<!-- BUG-021: edit mode -->
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
												onclick={() => { editingId = null; editBody = ''; }}
												class="text-xs font-medium underline"
											>
												{t('common.cancel', 'Cancel')}
											</button>
										</div>
									{:else}
										<p class="whitespace-pre-wrap break-words">{msg.body}</p>
										<p class="mt-1 text-[10px] {mine ? 'text-[var(--color-primary-contrast)]/70' : 'text-[var(--color-muted)]'}">{new Date(msg.created_at).toLocaleTimeString()}</p>
										<!-- BUG-021: edit/delete actions on own messages -->
										{#if mine}
											<div class="mt-1 flex gap-2 opacity-0 transition-opacity group-hover:opacity-100">
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
											</div>
										{/if}
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
					<Send size={16} />
				</button>
			</form>
		{/if}
	</section>
</div>

<!-- BUG-022: new conversation dialog -->
{#if showNewConv}
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
		onclick={(e) => { if (e.target === e.currentTarget) showNewConv = false; }}
		onkeydown={(e) => { if (e.key === 'Escape') showNewConv = false; }}
		role="dialog"
		aria-modal="true"
		tabindex="-1"
	>
		<div class="w-full max-w-md rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6">
			<h2 class="mb-4 text-base font-semibold text-[var(--color-text)]">
				{t('messages.new_conversation', 'New conversation')}
			</h2>
			<form
				onsubmit={(e) => {
					e.preventDefault();
					startNewConversation();
				}}
				class="space-y-3"
			>
				<input
					type="text"
					bind:value={newRecipientId}
					placeholder={t('messages.recipient_id', 'Recipient user ID')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
				/>
				<LiteralSuggest
					textarea
					bind:value={newMessage}
					placeholder={t('messages.placeholder', 'Type a message…')}
					rows={3}
					wrapperClass="relative w-full"
					class="w-full resize-none rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
				/>
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
						disabled={creatingConv || !newRecipientId.trim() || !newMessage.trim()}
						class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
					>
						{t('messages.send', 'Send')}
					</button>
				</div>
			</form>
		</div>
	</div>
{/if}

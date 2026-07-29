<script lang="ts">
	import { untrack, onMount } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Send, MessageSquareOff, ArrowLeft } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { EmptyState } from '$lib/components/ui';
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
	// conversation's unread counter and last-message preview.
	onMount(() => {
		const handler = (ev: DMMessageEvent) => {
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
		// Clear the unread badge for the conversation the user just opened.
		conversations = conversations.map((c) =>
			c.id === id ? { ...c, unread_count: 0 } : c
		);
		void api.post(`/dm/conversations/${id}/read`);
	}

	async function sendMessage() {
		if (!activeId || !draft.trim() || sending) return;
		const body = draft.trim();
		draft = '';
		sending = true;
		const res = await api.post<{ id: string }>(`/dm/conversations/${activeId}/messages`, { body });
		sending = false;
		if (res.data) {
			messages = [
				...messages,
				{
					id: res.data.id,
					sender_id: page.data.user?.id ?? '',
					sender_username: page.data.user?.username ?? '',
					sender_display_name: page.data.user?.display_name ?? '',
					body,
					created_at: new Date().toISOString(),
					read_at: null
				}
			];
		} else {
			draft = body; // restore on failure
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

<div class="mx-auto flex w-full max-w-4xl gap-0">
	<!-- Conversation list -->
	<aside class="flex w-full max-w-xs shrink-0 flex-col border-r border-[var(--color-border)]" class:hidden={!!activeId && conversations.length > 0}>
		<div class="border-b border-[var(--color-border)] px-4 py-3">
			<h1 class="text-base font-semibold text-[var(--color-text)]">{t('messages.heading', 'Messages')}</h1>
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
							<li class="flex {mine ? 'justify-end' : 'justify-start'}">
								<div class="max-w-[75%] rounded-[var(--radius)] px-3 py-2 text-sm {mine ? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]' : 'border border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-text)]'}">
									<p class="whitespace-pre-wrap break-words">{msg.body}</p>
									<p class="mt-1 text-[10px] {mine ? 'text-[var(--color-primary-contrast)]/70' : 'text-[var(--color-muted)]'}">{new Date(msg.created_at).toLocaleTimeString()}</p>
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
				<input
					type="text"
					bind:value={draft}
					placeholder={t('messages.placeholder', 'Type a message…')}
					class="flex-1 rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
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

<script lang="ts">
	// DM thread pane for the Communications Hub — extracted from the original
	// /messages 2-pane view. Adds the presence line, the live typing
	// indicator (Section 10 relay via realtime.sendTyping), read receipts,
	// the call-request entry and the details-rail toggle.
	//
	// Ref: japaconnect-engineering/sections/48-direct-messages.md
	import { untrack, tick } from 'svelte';
	import { t } from '#lib/i18n/index.js';
	import { page } from '$app/state';
	import {
		Send,
		ArrowLeft,
		Pencil,
		Trash2,
		Loader2,
		Flag,
		Package,
		Phone,
		Video,
		PanelRight,
		Lock
	} from 'lucide-svelte';
	import { api } from '#lib/api/client.js';
	import { LiteralSuggest } from '#lib/components/ui/index.js';
	import VerifiedBadge from '#lib/components/ui/VerifiedBadge.svelte';
	import ReportDialog from '#lib/components/profile/ReportDialog.svelte';
	import CallRequestDialog from './CallRequestDialog.svelte';
	import { realtime, type DMMessageEvent, type TypingEvent } from '#lib/realtime/realtime.svelte.js';
	import { remoteImageUrl } from '#lib/utils/remoteImage.js';
	import type { DMConversation, DMMessage } from '#lib/types/index.js';

	interface Props {
		conversation: DMConversation;
		initialMessages: DMMessage[];
		initialCursor: string | null;
		pageSize: number;
		onBack: () => void;
		onToggleDetails: () => void;
		/** Called after this thread marks the conversation read — the
		    stream row clears its unread badge. */
		onRead?: (conversationId: string) => void;
	}

	let {
		conversation,
		initialMessages,
		initialCursor,
		pageSize,
		onBack,
		onToggleDetails,
		onRead
	}: Props = $props();

	let messages = $state<DMMessage[]>(untrack(() => initialMessages));
	let msgsCursor = $state<string | null>(untrack(() => initialCursor));
	let loadingEarlier = $state(false);
	let draft = $state('');
	let sending = $state(false);
	let threadEl = $state<HTMLElement | null>(null);
	let lastReadMarked = $state<string | null>(null);

	// Edit/delete message state (15-min window enforced server-side).
	let editingId = $state<string | null>(null);
	let editBody = $state('');
	let savingEdit = $state(false);
	let deletingId = $state<string | null>(null);
	// Edit/delete failures — the silent paths used to discard the action
	// with no feedback. Shown above the composer.
	let actionError = $state('');

	// Report incoming message (Section 51 via the DM report route).
	let reportingMsgId = $state<string | null>(null);
	let reportDialogOpen = $state(false);

	// Call-request dialog — no call API exists; it posts a real proposal
	// message into this thread and appends the bubble on success.
	let callDialogOpen = $state(false);
	let callKind = $state<'voice' | 'video'>('voice');

	// Typing indicator — the other party's signal expires after 4s in case
	// the is_typing=false relay was lost (server throttles at ~3s).
	let peerTyping = $state(false);
	let peerTypingTimer: ReturnType<typeof setTimeout> | undefined;
	// Outbound typing signal — send at most once per 2s while composing.
	let typingSentAt = 0;

	const isOrg = $derived(conversation.conversation_type === 'org');
	const online = $derived(
		!isOrg && realtime.onlineUserIds.has(conversation.other_user_id)
	);

	// convId tracks the id STRING, not the conversation prop object — the
	// effects below must only re-run on a true conversation switch, never
	// when a parent state update hands back a fresh object with the same id
	// (a re-run resets draft and lastReadMarked).
	const convId = $derived(conversation.id);

	// Re-sync when the selected conversation changes (SvelteKit reuses the
	// component across sel= navigations).
	$effect(() => {
		convId;
		messages = untrack(() => initialMessages);
		msgsCursor = untrack(() => initialCursor);
		draft = '';
		editingId = null;
		peerTyping = false;
		lastReadMarked = null;
		actionError = '';
		tick().then(() => {
			if (threadEl) threadEl.scrollTop = threadEl.scrollHeight;
		});
	});

	// Mark read once the payload is in — keeps the server unread count right.
	$effect(() => {
		const id = convId;
		if (lastReadMarked === id) return;
		lastReadMarked = id;
		void api.post(`/dm/conversations/${id}/read`).then((res) => {
			if (!res.error) onRead?.(id);
		});
	});

	// Live DM delivery + typing relay. Events from the current user are
	// skipped — the optimistic append in sendMessage already added them.
	$effect(() => {
		const onDM = (ev: DMMessageEvent) => {
			if (ev.sender_id === page.data.user?.id) return;
			if (ev.conversation_id !== conversation.id) return;
			peerTyping = false;
			messages = [
				...messages,
				{
					// ev.id is the stored row id on newer servers — report/
					// delete need it; the randomUUID is the deploy-drift fallback.
					id: ev.id ?? crypto.randomUUID(),
					sender_id: ev.sender_id,
					sender_username: '',
					sender_display_name: ev.sender_display_name ?? '',
					sender_verified: ev.sender_verified,
					sender_verification_type: ev.sender_verification_type ?? null,
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
			void api.post(`/dm/conversations/${ev.conversation_id}/read`).then((res) => {
				if (!res.error) onRead?.(ev.conversation_id);
			});
			tick().then(() => {
				if (threadEl) threadEl.scrollTop = threadEl.scrollHeight;
			});
		};
		const onTyping = (ev: TypingEvent) => {
			if (ev.conversation_id !== conversation.id) return;
			if (ev.user_id === page.data.user?.id) return;
			clearTimeout(peerTypingTimer);
			peerTyping = ev.is_typing;
			if (ev.is_typing) {
				peerTypingTimer = setTimeout(() => (peerTyping = false), 4000);
			}
		};
		realtime.onDMMessage(onDM);
		realtime.onTyping(onTyping);
		return () => {
			realtime.offDMMessage(onDM);
			realtime.offTyping(onTyping);
			clearTimeout(peerTypingTimer);
		};
	});

	function onDraftInput(_v?: string) {
		const now = Date.now();
		if (now - typingSentAt < 2000) return;
		typingSentAt = now;
		realtime.sendTyping(conversation.id, true);
	}

	async function loadEarlierMessages() {
		if (loadingEarlier || !msgsCursor) return;
		loadingEarlier = true;
		// Record the offset from the top of the loaded history so the
		// prepend doesn't visually jump — after the older page mounts, the
		// scroll position is restored to the same message.
		const prevHeight = threadEl?.scrollHeight ?? 0;
		const prevTop = threadEl?.scrollTop ?? 0;
		const res = await api.getEnvelope<DMMessage[]>(
			`/dm/conversations/${conversation.id}/messages?page_size=${pageSize}&cursor=${encodeURIComponent(msgsCursor)}`
		);
		loadingEarlier = false;
		const items = res.data ?? [];
		msgsCursor = (res.envelope?.next_cursor as string | null | undefined) ?? null;
		messages = [...items.reverse(), ...messages];
		await tick();
		if (threadEl) threadEl.scrollTop = threadEl.scrollHeight - prevHeight + prevTop;
	}

	async function sendMessage() {
		if (!draft.trim() || sending) return;
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
		const res = await api.post<DMMessage>(
			`/dm/conversations/${conversation.id}/messages`,
			{ body }
		);
		sending = false;
		if (res.error) {
			messages = messages.filter((m) => m.id !== tempId);
			draft = body;
		} else {
			// Swap the optimistic bubble for the stored row — the API
			// returns it, and edit/delete need the real message id.
			if (res.data?.id) {
				messages = messages.map((m) => (m.id === tempId ? { ...m, ...res.data } : m));
			}
			realtime.sendTyping(conversation.id, false);
		}
	}

	// Editable only inside the server's 15-minute window (dm.go EditMessage)
	// — outside it the PATCH is rejected anyway, so the raw body isn't
	// fetched. Within it, walk raw pages until the row is found: the newest
	// 100 covers ~every real case; two extra pages guard busy threads.
	const EDIT_WINDOW_MS = 15 * 60 * 1000;

	async function startEdit(msg: DMMessage) {
		editingId = msg.id;
		editBody = msg.body;
		actionError = '';
		// The raw (unresolved) body keeps hand-typed {{...}} literals alive
		// across a re-save — the display payload has them already resolved.
		if (Date.now() - Date.parse(msg.created_at) > EDIT_WINDOW_MS) return;
		let cursor: string | null = null;
		for (let pageNum = 0; pageNum < 3; pageNum++) {
			const qs: string = cursor ? `&cursor=${encodeURIComponent(cursor)}` : '';
			const res = await api.getEnvelope<DMMessage[]>(
				`/dm/conversations/${conversation.id}/messages?raw=1&page_size=100${qs}`
			);
			if (editingId !== msg.id) return;
			const raw = res.data?.find((m) => m.id === msg.id);
			if (raw) {
				editBody = raw.body;
				return;
			}
			cursor = (res.envelope?.next_cursor as string | null | undefined) ?? null;
			if (!cursor || !res.data?.length) return;
		}
	}

	async function saveEdit(msg: DMMessage) {
		if (!editBody.trim() || editingId !== msg.id || savingEdit) return;
		savingEdit = true;
		actionError = '';
		const res = await api.patch(
			`/dm/conversations/${conversation.id}/messages/${msg.id}`,
			{ body: editBody.trim() }
		);
		savingEdit = false;
		if (res.error) {
			// Keep the editor open so the edit isn't lost — e.g. when the
			// server rejects it for being outside the 15-minute window.
			actionError =
				res.error.message ?? t('messages.save_failed', 'Could not save the message.');
			return;
		}
		messages = messages.map((m) =>
			m.id === msg.id ? { ...m, body: editBody.trim() } : m
		);
		editingId = null;
		editBody = '';
	}

	async function deleteMessage(msg: DMMessage) {
		if (deletingId) return;
		deletingId = msg.id;
		actionError = '';
		const res = await api.delete(
			`/dm/conversations/${conversation.id}/messages/${msg.id}`
		);
		deletingId = null;
		if (res.error) {
			actionError =
				res.error.message ?? t('messages.delete_failed', 'Could not delete the message.');
			return;
		}
		messages = messages.filter((m) => m.id !== msg.id);
	}

	/** Read receipt — the last outgoing message's read_at drives "Seen". */
	const lastSeen = $derived.by(() => {
		const mine = page.data.user?.id;
		for (let i = messages.length - 1; i >= 0; i--) {
			const m = messages[i];
			if (m.sender_id === mine) return m.read_at ? m : null;
		}
		return null;
	});

	const profileHref = $derived(
		isOrg ? `/org/${conversation.other_username}` : `/u/${conversation.other_username}`
	);

	// Day-divider helpers (mockup: "Today, 10:24 AM" separators between
	// message groups).
	function dayKey(iso: string): string {
		const d = new Date(iso);
		return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`;
	}
	function dayLabel(iso: string): string {
		if (dayKey(iso) === dayKey(new Date().toISOString())) {
			return t('hub.today', 'Today');
		}
		return new Date(iso).toLocaleDateString();
	}
</script>

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
	<!-- Thread header — counterpart + presence + call/details actions. -->
	<div class="flex items-center gap-3 border-b border-[var(--color-border)] px-4 py-3">
		<button
			type="button"
			onclick={onBack}
			class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] md:hidden"
			aria-label={t('messages.back', 'Back')}
		>
			<ArrowLeft class="rtl:-scale-x-100" size={18} />
		</button>
		{#if conversation.other_avatar_url}
			<img
				src={remoteImageUrl(conversation.other_avatar_url)}
				alt=""
				class="h-9 w-9 rounded-full object-cover"
			/>
		{:else}
			<span
				class="grid h-9 w-9 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]"
				>{(conversation.other_display_name ?? '?').charAt(0).toUpperCase()}</span
			>
		{/if}
		<div class="min-w-0 flex-1">
			<p class="flex items-center gap-1 truncate text-sm font-medium text-[var(--color-text)]">
				<span class="truncate">{conversation.other_display_name}</span>
				<VerifiedBadge
					verified={conversation.other_verified}
					type={conversation.other_verification_type}
					scope={isOrg ? 'organization' : 'user'}
					size={14}
				/>
			</p>
			<p class="truncate text-xs text-[var(--color-muted)]">
				{#if peerTyping}
					<span class="text-[var(--color-primary-text)]">{t('hub.typing', 'typing…')}</span>
				{:else if online}
					{t('hub.online', 'Online')}
				{:else if conversation.other_username}
					<a href={profileHref} class="hover:text-[var(--color-primary-text)]"
						>@{conversation.other_username}</a
					>
				{/if}
			</p>
		</div>
		<!-- Call entry points — no call API exists (gap); the dialog posts a
		     real call-proposal message into the thread instead. -->
		<button
			type="button"
			onclick={() => {
				callKind = 'voice';
				callDialogOpen = true;
			}}
			class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
			aria-label={t('hub.call_voice', 'Request a voice call')}
			title={t('hub.call_voice', 'Request a voice call')}
		>
			<Phone size={16} />
		</button>
		<button
			type="button"
			onclick={() => {
				callKind = 'video';
				callDialogOpen = true;
			}}
			class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
			aria-label={t('hub.call_video', 'Request a video call')}
			title={t('hub.call_video', 'Request a video call')}
		>
			<Video size={16} />
		</button>
		<button
			type="button"
			onclick={onToggleDetails}
			class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
			aria-label={t('hub.details_toggle', 'Conversation details')}
			title={t('hub.details_toggle', 'Conversation details')}
		>
			<PanelRight size={16} />
		</button>
	</div>

	<!-- Conversation-kind line (mockup: "Direct message · Member
	     conversation" caption under the header). -->
	<p class="border-b border-[var(--color-border)] px-4 py-1.5 text-center text-[11px] text-[var(--color-muted)]">
		{isOrg
			? t('hub.thread_kind_org', 'Org inbox · Services conversation')
			: t('hub.thread_kind_dm', 'Direct message · Member conversation')}
	</p>

	<!-- Messages -->
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
				{#each messages as msg, msgIdx (msg.id)}
					{@const prev = msgIdx > 0 ? messages[msgIdx - 1] : null}
					{#if !prev || dayKey(prev.created_at) !== dayKey(msg.created_at)}
						<li class="my-2 text-center">
							<span
								class="rounded-full bg-[var(--color-surface-subtle)] px-3 py-1 text-[11px] text-[var(--color-muted)]"
							>
								{dayLabel(msg.created_at)}
							</span>
						</li>
					{/if}
					{@const mine = msg.sender_id === page.data.user?.id}
					<li class="group flex {mine ? 'justify-end' : 'justify-start'}">
						{#if !mine}
							{#if conversation.other_avatar_url}
								<img
									src={remoteImageUrl(conversation.other_avatar_url)}
									alt=""
									class="me-1.5 mt-auto h-6 w-6 shrink-0 rounded-full object-cover"
								/>
							{:else}
								<span
									class="me-1.5 mt-auto grid h-6 w-6 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[10px] font-medium text-[var(--color-text-subtle)]"
									>{(conversation.other_display_name ?? '?')
										.charAt(0)
										.toUpperCase()}</span
								>
							{/if}
						{/if}
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
										disabled={savingEdit}
										class="text-xs font-medium underline disabled:opacity-50"
									>
										{savingEdit ? t('common.saving', 'Saving…') : t('messages.save', 'Save')}
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
											: 'border-[var(--color-border)] bg-[var(--color-surface-subtle)] text-[var(--color-primary-text)]'}"
									>
										<Package size={13} aria-hidden="true" />
										<span class="truncate font-medium">{msg.context_label ?? msg.context_slug}</span>
									</a>
								{/if}
								{#if msg.sent_as_org && msg.sender_display_name}
									<p
										class="mb-0.5 text-[10px] font-medium {mine
											? 'text-[var(--color-primary-contrast)]/80'
											: 'text-[var(--color-text-subtle)]'}"
									>
										{msg.sender_display_name}
										{#if msg.sender_verified}
											<VerifiedBadge verified type={msg.sender_verification_type} />
										{/if}
									</p>
								{/if}
								<p class="whitespace-pre-wrap break-words">{msg.body}</p>
								<p
									class="mt-1 flex items-center gap-1.5 text-[10px] {mine
										? 'text-[var(--color-primary-contrast)]/70'
										: 'text-[var(--color-muted)]'}"
								>
									{new Date(msg.created_at).toLocaleTimeString()}
									{#if mine && msg === lastSeen}
										· {t('hub.seen', 'Seen')}
									{/if}
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
		{#if peerTyping}
			<div class="mt-2 flex justify-start" aria-live="polite">
				<span
					class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2"
				>
					<span class="h-1.5 w-1.5 animate-pulse rounded-full bg-[var(--color-muted)]"></span>
					<span class="h-1.5 w-1.5 animate-pulse rounded-full bg-[var(--color-muted)] [animation-delay:150ms]"></span>
					<span class="h-1.5 w-1.5 animate-pulse rounded-full bg-[var(--color-muted)] [animation-delay:300ms]"></span>
				</span>
			</div>
		{/if}
	</div>

	{#if actionError}
		<p class="px-4 pb-1 text-xs text-[var(--color-danger)]" role="alert">{actionError}</p>
	{/if}

	<!-- Composer — typing signals fire on input (2s throttle; the server
	     enforces ~3s). No file attach: dm_messages has no media_id column
	     yet — attachments live on support tickets for now. -->
	<form
		class="flex items-center gap-2 border-t border-[var(--color-border)] p-3 pe-14"
		onsubmit={(e) => {
			e.preventDefault();
			sendMessage();
		}}
	>
		<LiteralSuggest
			bind:value={draft}
			onvalue={onDraftInput}
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
	<!-- Reassurance caption mirroring the mockup's footer line — kept
	     honest (no E2E encryption exists, so no such claim). -->
	<p
		class="flex items-center justify-center gap-1.5 border-t border-[var(--color-border)] px-4 py-1.5 text-[11px] text-[var(--color-muted)]"
	>
		<Lock size={11} aria-hidden="true" />
		{isOrg
			? t('hub.private_note_org', 'Only members of this inbox can see this conversation.')
			: t('hub.private_note', 'This is a private conversation.')}
	</p>
</div>

{#if reportingMsgId}
	<ReportDialog
		bind:open={reportDialogOpen}
		entityType="dm_message"
		entityId={reportingMsgId}
		dmConversationId={conversation.id}
		entityName={conversation.other_display_name ?? t('messages.report', 'Report')}
	/>
{/if}

<CallRequestDialog
	bind:open={callDialogOpen}
	{conversation}
	kind={callKind}
	onSent={(body, msg) => {
		messages = [
			...messages,
			{
				id: msg?.id ?? crypto.randomUUID(),
				sender_id: page.data.user?.id ?? '',
				sender_username: page.data.user?.username ?? '',
				sender_display_name: page.data.user?.display_name ?? '',
				body,
				created_at: msg?.created_at ?? new Date().toISOString(),
				read_at: null
			}
		];
		tick().then(() => {
			if (threadEl) threadEl.scrollTop = threadEl.scrollHeight;
		});
	}}
/>

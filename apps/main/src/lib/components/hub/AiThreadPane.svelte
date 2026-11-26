<script lang="ts">
	// Japa AI thread pane for the Communications Hub — transcript of a unified
	// AI conversation (GET /ai/conversations/{id}) plus a compact composer
	// that POSTs /ai/chat in JSON mode ({reply, conversation_id}). The full
	// tool/agent surface stays at /ai — the header links there (ADR-130
	// ?c= deep link).
	//
	// Ref: japaconnect-engineering/adr/ADR-130-unified-japa-ai.md
	import { untrack } from 'svelte';
	import { t } from '#lib/i18n/index.js';
	import { api } from '#lib/api/client.js';
	import { ArrowLeft, Send, Loader2, Sparkles, PanelRight, ArrowUpRight } from 'lucide-svelte';
	import type { ConversationMessage } from '@japaconnect/ai-panel';

	interface AiDetail {
		conversation_id: string;
		title?: string;
		messages: ConversationMessage[];
		created_at: string;
		updated_at: string;
		running_session_id?: string;
	}

	interface Props {
		conversationId: string;
		onBack: () => void;
		onToggleDetails: () => void;
	}

	let { conversationId, onBack, onToggleDetails }: Props = $props();

	let messages = $state<ConversationMessage[]>([]);
	let title = $state('');
	let loading = $state(true);
	let loadFailed = $state(false);
	let draft = $state('');
	let sending = $state(false);
	let sendError = $state('');

	// Load the transcript whenever the selected AI thread changes.
	$effect(() => {
		const id = conversationId;
		loading = true;
		loadFailed = false;
		messages = [];
		void api
			.get<AiDetail>(`/ai/conversations/${encodeURIComponent(id)}`)
			.then((res) => {
				if (conversationId !== id) return;
				if (res.data) {
					messages = res.data.messages ?? [];
					title = res.data.title ?? '';
				} else {
					loadFailed = true;
				}
				loading = false;
			})
			.catch(() => {
				// Network-level failure (api.get only resolves on HTTP replies)
				// — without this the spinner would hang forever.
				if (conversationId !== id) return;
				loadFailed = true;
				loading = false;
			});
	});

	async function send() {
		const body = draft.trim();
		if (!body || sending) return;
		draft = '';
		sending = true;
		sendError = '';
		// Optimistic user turn — replaced by the next transcript load.
		messages = [
			...messages,
			{
				id: crypto.randomUUID(),
				conversation_id: conversationId,
				role: 'user',
				content: body,
				created_at: new Date().toISOString()
			}
		];
		const res = await api.post<{ reply: string; conversation_id: string }>('/ai/chat', {
			message: body,
			conversation_id: conversationId
		});
		sending = false;
		if (res.data) {
			messages = [
				...messages,
				{
					id: crypto.randomUUID(),
					conversation_id: conversationId,
					role: 'assistant',
					content: res.data.reply,
					created_at: new Date().toISOString()
				}
			];
		} else {
			sendError = res.error?.message ?? t('hub.ai_send_error', 'The AI could not respond.');
			draft = body;
		}
	}
</script>

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
	<div class="flex items-center gap-3 border-b border-[var(--color-border)] px-4 py-3">
		<button
			type="button"
			onclick={onBack}
			class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] md:hidden"
			aria-label={t('messages.back', 'Back')}
		>
			<ArrowLeft class="rtl:-scale-x-100" size={18} />
		</button>
		<span
			class="grid h-9 w-9 place-items-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary-text)]"
		>
			<Sparkles size={17} aria-hidden="true" />
		</span>
		<div class="min-w-0 flex-1">
			<p class="truncate text-sm font-medium text-[var(--color-text)]">
				{title || t('hub.ai_fallback_title', 'Japa AI')}
			</p>
			<p class="truncate text-xs text-[var(--color-muted)]">
				{t('hub.ai_subtitle', 'AI assistant')}
			</p>
		</div>
		<a
			href="/ai?c={conversationId}"
			class="inline-flex shrink-0 items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-2.5 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
			title={t('hub.ai_open_full', 'Open in Japa AI')}
		>
			<ArrowUpRight size={14} aria-hidden="true" />
			{t('hub.ai_open_full', 'Open in Japa AI')}
		</a>
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

	<div class="min-h-0 flex-1 overflow-y-auto p-4">
		{#if loading}
			<div class="grid place-items-center py-12">
				<Loader2 size={22} class="animate-spin text-[var(--color-muted)]" />
			</div>
		{:else if loadFailed}
			<p class="py-8 text-center text-sm text-[var(--color-muted)]">
				{t('hub.ai_load_error', 'Could not load this conversation.')}
			</p>
		{:else if messages.length === 0}
			<p class="py-8 text-center text-sm text-[var(--color-muted)]">
				{t('hub.ai_empty', 'Ask Japa AI anything about your move.')}
			</p>
		{:else}
			<ul class="space-y-2">
				{#each messages as msg (msg.id)}
					{@const mine = msg.role === 'user'}
					<li class="flex {mine ? 'justify-end' : 'justify-start'}">
						<div
							class="max-w-[80%] rounded-[var(--radius)] px-3 py-2 text-sm {mine
								? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
								: 'border border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-text)]'}"
						>
							<p class="whitespace-pre-wrap break-words">{msg.content}</p>
						</div>
					</li>
				{/each}
			</ul>
		{/if}
	</div>

	<form
		class="flex items-center gap-2 border-t border-[var(--color-border)] p-3 pe-14"
		onsubmit={(e) => {
			e.preventDefault();
			send();
		}}
	>
		<input
			type="text"
			bind:value={draft}
			placeholder={t('hub.ai_placeholder', 'Ask Japa AI…')}
			class="w-full rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
		/>
		<button
			type="submit"
			disabled={sending || !draft.trim()}
			class="grid h-9 w-9 shrink-0 place-items-center rounded-full bg-[var(--color-primary)] text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
			aria-label={t('messages.send', 'Send')}
		>
			{#if sending}<Loader2 size={16} class="animate-spin" />{:else}<Send
					class="rtl:-scale-x-100"
					size={16}
				/>{/if}
		</button>
	</form>
	{#if sendError}
		<p class="px-4 pb-2 text-sm text-[var(--color-danger)]" role="alert">{sendError}</p>
	{/if}
</div>

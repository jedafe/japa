<script lang="ts">
	import { t } from '$lib/i18n';
	import { tick } from 'svelte';
	import { Send, Sparkles, MessageSquare, Loader2, AlertCircle, ShieldCheck, Menu, X, Plus } from 'lucide-svelte';
	import { enhance } from '$app/forms';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import AiModelVoicePicker from './AiModelVoicePicker.svelte';
	import AiSourceCitations from './AiSourceCitations.svelte';
	import AiTypingDots from './AiTypingDots.svelte';
	import type { AiCitationSource } from './types';

	/**
	 * JapaBot chat — conversation sidebar, RAG-cited transcript, and composer
	 * with model/voice pickers. Sends via the hosting route's ?/send form
	 * action so the component can live on any page (canonical home: /ai).
	 *
	 * Ref: japaconnect-engineering/sections/20-ai-core.md
	 *      japaconnect-engineering/sections/69-rag-knowledge-layer.md
	 */
	interface ConversationSummary {
		conversation_id: string;
		preview: string;
		turn_count: number;
		last_entry_at: string;
	}

	interface Props {
		conversations: ConversationSummary[];
		models: { id: string; label: string }[];
		voices: { id: string; name: string }[];
	}

	let { conversations, models, voices }: Props = $props();

	type ChatMessage = {
		role: 'user' | 'assistant';
		content: string;
		conversation_id?: string;
		sources?: AiCitationSource[];
	};

	type ConversationEntry = {
		id: string;
		content: string;
		metadata?: Record<string, unknown>;
		created_at: string;
	};

	let messages = $state<ChatMessage[]>([]);
	let input = $state('');
	let inputEl: HTMLTextAreaElement | undefined = $state();

	function resizeInput() {
		if (!inputEl) return;
		inputEl.style.height = 'auto';
		inputEl.style.height = Math.min(inputEl.scrollHeight, 160) + 'px';
	}

	$effect(() => {
		// Grow/shrink the composer with its content (incl. reset after send).
		// Measure after paint — on mount the flex width may not be resolved.
		input;
		const id = requestAnimationFrame(resizeInput);
		window.addEventListener('resize', resizeInput);
		return () => {
			cancelAnimationFrame(id);
			window.removeEventListener('resize', resizeInput);
		};
	});
	let conversationId = $state<string>('');
	let sending = $state(false);
	let error = $state('');
	let scrollEl: HTMLElement | undefined = $state();
	let loadingConversation = $state(false);
	let sidebarOpen = $state(false);

	// Composer pickers — the lists come from the server load (GET /ai/models
	// + /ai/voices); an empty list hides the picker and the backend applies
	// its configured default.
	let selectedModel = $state('');
	let selectedVoice = $state('');
	let composerForm: HTMLFormElement | undefined = $state();
	// The backend already emits an empty-id "Default" entry for models
	// (/ai/models), so only prepend our own when none exists — two value:''
	// options would collide on the picker's __none__ key.
	const modelOptions = $derived([
		...(models.some((m) => m.id === '')
			? []
			: [{ value: '', label: t('ai.chat.model_default', 'Default model') }]),
		...models.map((m) => ({ value: m.id, label: m.label }))
	]);
	const voiceOptions = $derived([
		...(voices.some((v) => v.id === '')
			? []
			: [{ value: '', label: t('ai.chat.voice_default', 'Default voice') }]),
		...voices.map((v) => ({ value: v.id, label: v.name }))
	]);

	// Suggestion prompts shown on the empty transcript — UI copy only.
	const suggestions = $derived([
		t('ai.chat.suggestion_1', 'What documents do I need for a UK student visa?'),
		t('ai.chat.suggestion_2', 'How do I find scholarships for masters?'),
		t('ai.chat.suggestion_3', "What's the cost of living in Toronto vs Lagos?"),
		t('ai.chat.suggestion_4', 'How do I send money home cheaply?')
	]);

	// Auto-scroll to the latest message whenever the transcript grows.
	$effect(() => {
		void messages.length;
		if (scrollEl) {
			scrollEl.scrollTop = scrollEl.scrollHeight;
		}
	});

	function formatRole(metadata?: Record<string, unknown>): 'user' | 'assistant' {
		if (!metadata || typeof metadata.role !== 'string') return 'assistant';
		return metadata.role === 'user' ? 'user' : 'assistant';
	}

	async function loadConversation(conversationIdToLoad: string) {
		loadingConversation = true;
		const res = await api.get<{ conversation_id: string; entries: ConversationEntry[]; turn_count: number }>(
			`/ai/conversations/${conversationIdToLoad}`
		);
		loadingConversation = false;
		if (!res.error && res.data) {
			conversationId = res.data.conversation_id;
			error = '';
			messages = (res.data.entries ?? []).map((e) => ({
				role: formatRole(e.metadata),
				content: e.content,
				conversation_id: e.id
			}));
			sidebarOpen = false;
		} else if (res.error) {
			error = res.error.message;
		}
	}

	function startNewChat() {
		conversationId = '';
		messages = [];
		sidebarOpen = false;
	}
</script>

<div class="mx-auto flex min-h-0 w-full max-w-5xl flex-1 flex-col gap-4 lg:flex-row">
	<!-- Conversation sidebar -->
	<aside
		class="w-full lg:w-64 lg:shrink-0 {sidebarOpen
			? 'flex'
			: 'hidden lg:flex'} max-h-[50vh] lg:max-h-full flex-col rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]"
	>
		<div class="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
			<h2 class="text-sm font-semibold text-[var(--color-text)]">{t('ai.chat.history', 'Conversations')}</h2>
			<button
				type="button"
				onclick={startNewChat}
				class="inline-flex items-center gap-1 text-xs text-[var(--color-primary)] hover:underline"
			>
				<Plus size={14} />
				{t('ai.chat.new', 'New')}
			</button>
		</div>
		<div class="flex-1 overflow-y-auto p-2">
			{#if conversations.length === 0}
				<p class="p-2 text-center text-xs text-[var(--color-muted)]">{t('ai.chat.no_history', 'No previous conversations.')}</p>
			{:else}
				<ul class="space-y-1">
					{#each conversations as conv (conv.conversation_id)}
						<li>
							<button
								type="button"
								onclick={() => loadConversation(conv.conversation_id)}
								class="w-full rounded-[var(--radius)] px-3 py-2 text-left text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] {conversationId === conv.conversation_id ? 'bg-[var(--color-primary)]/10 font-medium' : ''}"
							>
								<div class="truncate">{conv.preview || t('ai.chat.unnamed', 'Conversation')}</div>
								<div class="mt-0.5 text-xs text-[var(--color-muted)]">
									{conv.turn_count} {conv.turn_count === 1 ? t('ai.chat.turn', 'turn') : t('ai.chat.turns', 'turns')}
								</div>
							</button>
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	</aside>

	<!-- Main chat — min-w-0 so the column shrinks below min-content width
	     (overflow into the right rail); min-h-0 so the transcript scrolls
	     instead of growing the page. -->
	<div class="flex min-w-0 min-h-0 flex-1 flex-col">
		<header class="mb-4 flex items-center gap-3">
			<button
				type="button"
				onclick={() => (sidebarOpen = !sidebarOpen)}
				class="mr-2 inline-flex items-center gap-1 text-sm text-[var(--color-muted)] lg:hidden"
				aria-label={t('ai.chat.toggle_history', 'Toggle conversation history')}
			>
				{#if sidebarOpen}
					<X size={18} />
				{:else}
					<Menu size={18} />
				{/if}
			</button>
			<div class="flex h-10 w-10 items-center justify-center rounded-full" style="background-color: var(--color-primary)">
				<Sparkles class="h-5 w-5 text-white" />
			</div>
			<div>
				<h1 class="text-xl font-bold tracking-tight">{t('ai.chat.title') ?? 'JapaBot'}</h1>
				<p class="text-xs" style="color: var(--color-muted)">{t('ai.chat.subtitle') ?? 'Your AI assistant'}</p>
			</div>
		</header>

		<!-- Transcript -->
		<div
			bind:this={scrollEl}
			class="relative flex-1 overflow-y-auto rounded-lg border p-4"
			style="border-color: var(--color-border); background-color: var(--color-surface)"
		>
			{#if messages.length === 0 && !loadingConversation}
				<div class="flex h-full flex-col items-center justify-center gap-2 p-6 text-center">
					<MessageSquare class="h-8 w-8" style="color: var(--color-muted)" />
					<p class="text-sm" style="color: var(--color-muted)">{t('ai.chat.empty') ?? 'Start a conversation with JapaBot'}</p>
					<div class="mt-3 grid w-full max-w-md grid-cols-1 gap-2 sm:grid-cols-2">
						{#each suggestions as suggestion (suggestion)}
							<button
								type="button"
								onclick={async () => {
									input = suggestion;
									// Let bind:value flush into the textarea before the
									// submit captures its DOM value.
									await tick();
									composerForm?.requestSubmit();
								}}
								class="rounded-lg border p-3 text-left text-sm transition-colors hover:border-[var(--color-primary)]"
								style="border-color: var(--color-border); background-color: var(--color-surface-subtle); color: var(--color-muted)"
							>
								{suggestion}
							</button>
						{/each}
					</div>
				</div>
			{:else}
				{#if loadingConversation}
					<div class="absolute inset-0 z-10 flex items-center justify-center bg-[var(--color-surface)]/80">
						<Loader2 class="h-6 w-6 animate-spin" style="color: var(--color-primary)" />
					</div>
				{/if}
				<div class="space-y-3">
					{#each messages as m}
						<div class="flex {m.role === 'user' ? 'justify-end' : 'justify-start'}">
							<div class="max-w-[80%]">
								<div
									class="rounded-2xl px-4 py-2 text-sm whitespace-pre-wrap"
									style={m.role === 'user'
										? 'background-color: var(--color-primary); color: white'
										: 'background-color: var(--color-muted-15); color: var(--color-text)'}
								>
									{m.content}
								</div>
								<!-- RAG source citations (Section 69, ADR-069) -->
								{#if m.role === 'assistant' && m.sources && m.sources.length > 0}
									<AiSourceCitations sources={m.sources} />
								{/if}
								<!-- Immigration/financial content disclaimer (Section 69) -->
								{#if m.role === 'assistant'}
									<p class="mt-1 flex items-center gap-1 text-xs" style="color: var(--color-muted)">
										<ShieldCheck size={11} />
										{t('ai.chat.disclaimer') ?? 'AI-generated. Verify immigration/financial details with official sources.'}
									</p>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</div>

		{#if error}
			<div class="mt-3 flex items-center gap-2 rounded-md border p-2 text-sm" style="border-color: var(--color-danger); background-color: var(--color-danger-bg)">
				<AlertCircle class="h-4 w-4" style="color: var(--color-danger)" />
				{error}
			</div>
		{/if}

		{#if sending}
			<AiTypingDots />
		{/if}

		<div class="mt-3">
			<AiModelVoicePicker
				bind:model={selectedModel}
				bind:voice={selectedVoice}
				{modelOptions}
				{voiceOptions}
				idPrefix="japabot"
			/>
		</div>

		<!-- Composer — posts to the hosting route's ?/send action -->
		<form
			bind:this={composerForm}
			method="POST"
			action="?/send"
			use:enhance={({ formData, cancel }) => {
				const submitted = (formData.get('message') as string | null)?.trim() ?? '';
				if (sending || !submitted) {
					cancel();
					return;
				}
				// Optimistically render the user bubble for every submission —
				// whether it came from Enter or the Send button.
				sending = true;
				error = '';
				messages = [...messages, { role: 'user', content: submitted }];
				input = '';
				return async ({ result }) => {
					sending = false;
					if (result.type === 'success' && result.data?.ok) {
						const replyData = result.data as { reply: string; conversation_id?: string; sources?: AiCitationSource[] };
						messages = [...messages, {
							role: 'assistant',
							content: replyData.reply,
							conversation_id: replyData.conversation_id,
							sources: replyData.sources
						}];
						if (replyData.conversation_id) conversationId = replyData.conversation_id;
						error = '';
					} else if (result.type === 'failure') {
						const failData = result.data as { error?: string; detail?: string; message?: string } | undefined;
						error = failData?.detail || failData?.error || t('ai.chat.error', 'JapaBot is unavailable');
						// The send failed — drop the optimistic user bubble and
						// restore the draft (the action echoes it back as
						// `message`) so a failure doesn't silently eat it.
						if (messages.length > 0 && messages[messages.length - 1].role === 'user') {
							messages = messages.slice(0, -1);
						}
						input = failData?.message ?? submitted;
					} else if (result.type === 'redirect') {
						// Custom enhance callbacks don't auto-follow redirects —
						// apply them (e.g. session expired → /login).
						await goto(result.location);
					} else if (result.type === 'error') {
						// Uncaught action error — same rollback as a fail().
						error = t('ai.chat.error', 'JapaBot is unavailable');
						if (messages.length > 0 && messages[messages.length - 1].role === 'user') {
							messages = messages.slice(0, -1);
						}
						input = submitted;
					}
				};
			}}
			class="mt-2 flex items-end gap-2"
		>
			<input type="hidden" name="conversation_id" value={conversationId} />
			<input type="hidden" name="model" value={selectedModel} />
			<input type="hidden" name="voice_id" value={selectedVoice} />
			<textarea
				name="message"
				bind:this={inputEl}
				bind:value={input}
				rows="1"
				placeholder={t('ai.chat.placeholder') ?? 'Ask JapaBot anything...'}
				oninput={resizeInput}
				class="flex-1 resize-none rounded-lg border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-background)"
				onkeydown={(e) => {
					if (e.key === 'Enter' && !e.shiftKey) {
						e.preventDefault();
						const form = (e.currentTarget as HTMLTextAreaElement).form;
						if (input.trim()) {
							form?.requestSubmit();
						}
					}
				}}
			></textarea>
			<button
				type="submit"
				disabled={sending || !input.trim()}
				class="flex h-10 w-10 items-center justify-center rounded-lg text-white transition-colors disabled:opacity-50"
				style="background-color: var(--color-primary)"
				aria-label={t('ai.chat.send') ?? 'Send'}
			>
				{#if sending}
					<Loader2 class="h-5 w-5 animate-spin" />
				{:else}
					<Send class="h-5 w-5" />
				{/if}
			</button>
		</form>
	</div>
</div>

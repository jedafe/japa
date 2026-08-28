<script lang="ts">
	import { t } from '$lib/i18n';
	import { untrack } from 'svelte';
	import { Send, Sparkles, MessageSquare, Loader2, AlertCircle, BookOpen, ShieldCheck, Menu, X, Plus } from 'lucide-svelte';
	import { enhance } from '$app/forms';
	import { api } from '$lib/api/client';
	import type { AiChatSource } from './+page.server';
	import type { PageData } from './$types';

	/**
	 * JapaBot chat with conversation history sidebar and detail loading.
	 *
	 * Ref: japaconnect-engineering/sections/20-ai-core.md
	 */
	let { data }: { data: PageData } = $props();

	type ChatMessage = {
		role: 'user' | 'assistant';
		content: string;
		conversation_id?: string;
		sources?: AiChatSource[];
	};

	type ConversationEntry = {
		id: string;
		content: string;
		metadata?: Record<string, unknown>;
		created_at: string;
	};

	let messages = $state<ChatMessage[]>([]);
	let input = $state('');
	let conversationId = $state<string>('');
	let sending = $state(false);
	let error = $state('');
	let scrollEl: HTMLElement | undefined = $state();
	let loadingConversation = $state(false);
	let sidebarOpen = $state(false);

	// Summaries from the server.
	let conversations = $state(untrack(() => data.conversations ?? []));

	// Auto-scroll to the latest message whenever the transcript grows.
	$effect(() => {
		void messages.length;
		if (scrollEl) {
			scrollEl.scrollTop = scrollEl.scrollHeight;
		}
	});

	function sourceHref(s: AiChatSource): string {
		if (!s.entity_id) return '#';
		return `/content/${s.entity_id}`;
	}

	function formatRole(metadata?: Record<string, unknown>): 'user' | 'assistant' {
		if (!metadata || typeof metadata.role !== 'string') return 'assistant';
		return metadata.role === 'user' ? 'user' : 'assistant';
	}

	async function loadConversation(conversationIdToLoad: string) {
		loadingConversation = true;
		const res = await api.get<{ conversation_id: string; entries: ConversationEntry[]; turn_count: number }>(
			`/api/v1/ai/conversations/${conversationIdToLoad}`
		);
		loadingConversation = false;
		if (!res.error && res.data) {
			conversationId = res.data.conversation_id;
			messages = (res.data.entries ?? []).map((e) => ({
				role: formatRole(e.metadata),
				content: e.content,
				conversation_id: e.id
			}));
			sidebarOpen = false;
		}
	}

	function startNewChat() {
		conversationId = '';
		messages = [];
		sidebarOpen = false;
	}
</script>

<svelte:head>
	<title>{t('ai.chat.title') ?? 'JapaBot Chat'} — JapaConnect</title>
</svelte:head>

<div class="mx-auto flex h-[calc(100vh-8rem)] max-w-5xl flex-col gap-4 lg:flex-row">
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

	<!-- Main chat -->
	<div class="flex flex-1 flex-col">
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
				<div class="flex h-full flex-col items-center justify-center gap-2 text-center">
					<MessageSquare class="h-8 w-8" style="color: var(--color-muted)" />
					<p class="text-sm" style="color: var(--color-muted)">{t('ai.chat.empty') ?? 'Start a conversation with JapaBot'}</p>
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
									<div class="mt-1.5 flex flex-wrap items-center gap-1.5">
										<span class="flex items-center gap-1 text-xs" style="color: var(--color-muted)">
											<BookOpen size={12} /> {t('ai.chat.sources') ?? 'Sources'}:
										</span>
										{#each m.sources as s, i (i)}
											<a
												href={sourceHref(s)}
												class="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs transition-colors hover:bg-[var(--color-surface-subtle)]"
												style="border-color: var(--color-border); color: var(--color-muted)"
											>
												{s.entity_type ?? 'content'}
												{#if s.similarity}
													<span style="color: var(--color-muted)">{Math.round(s.similarity * 100)}%</span>
												{/if}
											</a>
										{/each}
									</div>
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

		<!-- Composer -->
		<form
			method="POST"
			action="?/send"
			use:enhance={() => {
				return async ({ result }) => {
					sending = false;
					if (result.type === 'success' && result.data?.ok) {
						const replyData = result.data as { reply: string; conversation_id?: string; sources?: AiChatSource[] };
						messages = [...messages, {
							role: 'assistant',
							content: replyData.reply,
							conversation_id: replyData.conversation_id,
							sources: replyData.sources
						}];
						if (replyData.conversation_id) conversationId = replyData.conversation_id;
						error = '';
						input = '';
					} else if (result.type === 'failure') {
						error = (result.data as { error?: string })?.error ?? 'ai_error';
					}
				};
			}}
			class="mt-3 flex items-end gap-2"
		>
			<input type="hidden" name="conversation_id" value={conversationId} />
			<textarea
				name="message"
				bind:value={input}
				rows="1"
				placeholder={t('ai.chat.placeholder') ?? 'Ask JapaBot anything...'}
				class="flex-1 resize-none rounded-lg border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-background)"
				onkeydown={(e) => {
					if (e.key === 'Enter' && !e.shiftKey) {
						e.preventDefault();
						const form = (e.currentTarget as HTMLTextAreaElement).form;
						if (input.trim()) {
							sending = true;
							error = '';
							messages = [...messages, { role: 'user', content: input.trim() }];
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

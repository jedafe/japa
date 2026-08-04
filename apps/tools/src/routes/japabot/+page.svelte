<script lang="ts">
	import { api } from '$lib/api/client';
	import { mainAppUrl } from '$lib/seo';
	import { buildSeoMeta } from '$lib/seo';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import type { ChatMessage, ChatConversation, ChatSource } from '$lib/types';

	/**
	 * JapaBot standalone shell — §20, §69.
	 * Full-height chat UI without community surrounding. Answers are
	 * RAG-grounded (§69) — sourced from JapaConnect's published content.
	 * Auth required (ai.context.use permission) — guests see a sign-in prompt.
	 */
	let { data } = $props();

	let user = $state(data.user);
	let conversations = $state<ChatConversation[]>(data.conversations);

	let messages = $state<ChatMessage[]>([]);
	let input = $state('');
	let sending = $state(false);
	let activeConversationId = $state<string | null>(null);

	const seo = $derived(
		buildSeoMeta({
			title: 'JapaBot — Your AI Japa Guide | JapaConnect Tools',
			description:
				'Chat with JapaBot — answers grounded in JapaConnect\'s published guides, forum threads, and community knowledge via RAG.',
			path: '/japabot',
			origin: page.url.origin
		})
	);

	async function sendMessage() {
		if (!input.trim() || sending || !user) return;
		const userMessage: ChatMessage = {
			id: crypto.randomUUID(),
			role: 'user',
			content: input.trim(),
			created_at: new Date().toISOString()
		};
		messages = [...messages, userMessage];
		const messageText = input.trim();
		input = '';
		sending = true;

		const res = await api.post<{ reply: string; conversation_id: string; sources?: ChatSource[] }>(
			'/ai/chat',
			{ message: messageText, conversation_id: activeConversationId ?? undefined }
		);
		sending = false;

		if (res.error) {
			const errorMessage: ChatMessage = {
				id: crypto.randomUUID(),
				role: 'assistant',
				content: `Sorry, I couldn't process your request. ${res.error.message}`,
				created_at: new Date().toISOString()
			};
			messages = [...messages, errorMessage];
			return;
		}

		if (res.data) {
			activeConversationId = res.data.conversation_id;
			const assistantMessage: ChatMessage = {
				id: crypto.randomUUID(),
				role: 'assistant',
				content: res.data.reply,
				created_at: new Date().toISOString(),
				sources: res.data.sources
			};
			messages = [...messages, assistantMessage];
		}
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			sendMessage();
		}
	}

	function newConversation() {
		messages = [];
		activeConversationId = null;
	}

	async function loadConversation(conv: ChatConversation) {
		messages = conv.messages ?? [];
		activeConversationId = conv.id;
	}

	// Auto-scroll to bottom on new messages
	let messagesContainer: HTMLDivElement | null = $state(null);
	$effect(() => {
		if (messagesContainer) {
			messagesContainer.scrollTop = messagesContainer.scrollHeight;
		}
	});
</script>

<svelte:head>
	<title>{seo.title}</title>
	<meta name="description" content={seo.description} />
	<link rel="canonical" href={seo.canonical} />
	<meta property="og:title" content={seo.title} />
	<meta property="og:description" content={seo.description} />
	<meta property="og:url" content={seo.canonical} />
	<meta property="og:type" content="website" />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={seo.title} />
	<meta name="twitter:description" content={seo.description} />
</svelte:head>

<div class="flex flex-col h-[calc(100vh-var(--header-height)-64px)]">
	{#if !user}
		<!-- Sign-in required -->
		<div class="flex-1 flex items-center justify-center p-4">
			<div class="text-center max-w-sm">
				<div class="flex justify-center mb-4">
					<div class="w-16 h-16 rounded-2xl bg-[var(--tool-accent)] flex items-center justify-center">
						<Icon name="bot" size={32} class="text-white" />
					</div>
				</div>
				<h2 class="text-xl font-semibold text-text">JapaBot</h2>
				<p class="mt-2 text-sm text-text-subtle">
					Your AI guide for the japa journey. Ask about visas, scholarships, cost of living,
					and more — answers are grounded in JapaConnect's published content.
				</p>
				<div class="mt-5 flex flex-col gap-2">
					<a
						href={mainAppUrl('/login')}
						class="px-4 py-2.5 bg-[var(--tool-accent)] text-white rounded-lg text-sm font-medium hover:bg-[var(--tool-accent-hover)] transition-colors"
					>
						Sign in to chat
					</a>
					<a
						href={mainAppUrl('/signup')}
						class="px-4 py-2.5 text-sm font-medium text-text hover:text-[var(--tool-accent)] transition-colors"
					>
						Create free account
					</a>
				</div>
			</div>
		</div>
	{:else}
		<div class="flex flex-1 overflow-hidden">
			<!-- Conversation sidebar (desktop) -->
			{#if conversations.length > 0}
				<aside class="hidden md:flex flex-col w-64 border-r border-border bg-surface">
					<div class="p-3 border-b border-border">
						<button
							onclick={newConversation}
							class="w-full inline-flex items-center justify-center gap-2 px-3 py-2 bg-[var(--tool-accent)] text-white rounded-lg text-sm font-medium hover:bg-[var(--tool-accent-hover)] transition-colors"
						>
							<Icon name="plus" size={16} />
							New chat
						</button>
					</div>
					<div class="flex-1 overflow-y-auto p-2 space-y-1">
						{#each conversations as conv (conv.id)}
							<button
								onclick={() => loadConversation(conv)}
								class="w-full text-left px-3 py-2 text-sm rounded-lg transition-colors {activeConversationId === conv.id ? 'bg-surface-subtle text-text' : 'text-text-subtle hover:bg-surface-subtle'}"
							>
								{conv.title || 'New conversation'}
							</button>
						{/each}
					</div>
				</aside>
			{/if}

			<!-- Chat area -->
			<div class="flex-1 flex flex-col">
				<!-- Messages -->
				<div bind:this={messagesContainer} class="flex-1 overflow-y-auto px-4 py-6">
					<div class="max-w-2xl mx-auto space-y-4">
						{#if messages.length === 0}
							<div class="text-center py-12">
								<div class="flex justify-center mb-4">
									<div class="w-14 h-14 rounded-2xl bg-[var(--tool-accent)] flex items-center justify-center">
										<Icon name="bot" size={28} class="text-white" />
									</div>
								</div>
								<h2 class="text-lg font-semibold text-text">Ask JapaBot anything</h2>
								<p class="mt-1 text-sm text-text-subtle max-w-md mx-auto">
									Answers are grounded in JapaConnect's published guides, forum threads, and
									community knowledge.
								</p>
								<div class="mt-6 grid grid-cols-1 sm:grid-cols-2 gap-2 max-w-md mx-auto">
									{#each ['What documents do I need for a UK student visa?', 'How do I find scholarships for masters?', 'What\'s the cost of living in Toronto vs Lagos?', 'How do I send money home cheaply?'] as suggestion (suggestion)}
										<button
											onclick={() => { input = suggestion; sendMessage(); }}
											class="text-left p-3 text-sm bg-surface border border-border rounded-lg hover:border-[var(--tool-accent)] transition-colors text-text-subtle"
										>
											{suggestion}
										</button>
									{/each}
								</div>
							</div>
						{:else}
							{#each messages as msg (msg.id)}
								<div class="flex gap-3 {msg.role === 'user' ? 'flex-row-reverse' : ''}">
									<div
										class="flex-shrink-0 w-8 h-8 rounded-full flex items-center justify-center {msg.role === 'user' ? 'bg-primary text-primary-contrast' : 'bg-[var(--tool-accent)] text-white'}"
									>
										<Icon name={msg.role === 'user' ? 'user' : 'bot'} size={16} />
									</div>
									<div class="flex-1 {msg.role === 'user' ? 'text-right' : ''}">
										<div
											class="inline-block text-left px-4 py-3 rounded-lg text-sm {msg.role === 'user' ? 'bg-primary text-primary-contrast' : 'bg-surface border border-border text-text'}"
										>
											{msg.content}
										</div>
										{#if msg.sources && msg.sources.length > 0}
											<div class="mt-2 flex flex-wrap gap-1.5">
												<span class="text-xs text-muted">Sources:</span>
												{#each msg.sources as src (src.entity_id)}
													<a
														href={src.url}
														target="_blank"
														rel="noopener noreferrer"
														class="text-xs text-primary hover:text-primary-hover underline inline-flex items-center gap-0.5"
													>
														{src.title}
														<Icon name="external-link" size={10} />
													</a>
												{/each}
											</div>
										{/if}
									</div>
								</div>
							{/each}
							{#if sending}
								<div class="flex gap-3">
									<div class="flex-shrink-0 w-8 h-8 rounded-full bg-[var(--tool-accent)] flex items-center justify-center">
										<Icon name="bot" size={16} class="text-white" />
									</div>
									<div class="px-4 py-3 bg-surface border border-border rounded-lg">
										<div class="flex items-center gap-1.5">
											<span class="w-2 h-2 bg-muted rounded-full animate-bounce" style="animation-delay: 0ms"></span>
											<span class="w-2 h-2 bg-muted rounded-full animate-bounce" style="animation-delay: 150ms"></span>
											<span class="w-2 h-2 bg-muted rounded-full animate-bounce" style="animation-delay: 300ms"></span>
										</div>
									</div>
								</div>
							{/if}
						{/if}
					</div>
				</div>

				<!-- Input -->
				<div class="border-t border-border bg-surface p-4">
					<div class="max-w-2xl mx-auto flex items-end gap-2">
						<textarea
							bind:value={input}
							onkeydown={onKeydown}
							placeholder="Ask about visas, scholarships, cost of living..."
							rows="1"
							class="flex-1 px-4 py-3 bg-surface border border-border rounded-lg text-text resize-none focus:border-[var(--tool-accent)] focus:outline-none"
							style="min-height: 48px; max-height: 120px;"
						></textarea>
						<button
							onclick={sendMessage}
							disabled={!input.trim() || sending}
							class="flex-shrink-0 w-12 h-12 flex items-center justify-center bg-[var(--tool-accent)] text-white rounded-lg hover:bg-[var(--tool-accent-hover)] transition-colors disabled:opacity-50"
							aria-label="Send message"
						>
							{#if sending}
								<Icon name="loader-2" size={20} class="animate-spin" />
							{:else}
								<Icon name="send" size={20} />
							{/if}
						</button>
					</div>
				</div>
			</div>
		</div>
	{/if}
</div>

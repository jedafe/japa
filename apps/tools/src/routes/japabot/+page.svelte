<script lang="ts">
	import { untrack } from 'svelte';
	import { api } from '$lib/api/client';
	import { mainAppUrl, buildSeoMeta } from '$lib/seo';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import { t } from '$lib/i18n';
	import type {
		ChatMessage,
		ChatConversationSummary,
		ChatConversationDetail,
		ChatSource
	} from '$lib/types';

	/**
	 * JapaBot standalone shell — §20, §69.
	 * Full-height chat UI without community surrounding. Answers are
	 * RAG-grounded (§69) — sourced from JapaConnect's published content.
	 * Auth required (ai.context.use permission) — guests see a sign-in prompt.
	 */
	let { data } = $props();

	let user = $state(untrack(() => data.user));
	let conversations = $state<ChatConversationSummary[]>(untrack(() => data.conversations));
	let loadingConversation = $state(false);

	$effect(() => {
		if (!user) return;
		(async () => {
			// /ai/models already includes the empty-id "Default" option — use the
			// API list as-is and dedupe by id so the each-block keys stay unique.
			const modelRes = await api.get<{ id: string; label: string }[]>('/ai/models?category=chat');
			if (modelRes.data && modelRes.data.length > 0) {
				const seen = new Set<string>();
				models = modelRes.data.filter((m) => (seen.has(m.id) ? false : (seen.add(m.id), true)));
			}
			const voicesRes = await api.get<{ id: string; name: string }[]>('/ai/voices?page_size=100');
			if (voicesRes.data) {
				const seen = new Set<string>(['']);
				voices = [
					{ id: '', label: t('japabot.voice_default', 'Default voice') },
					...voicesRes.data
						.map((v) => ({ id: v.id, label: v.name }))
						.filter((v) => (seen.has(v.id) ? false : (seen.add(v.id), true)))
				];
			}
		})();
	});

	let messages = $state<ChatMessage[]>([]);
	let input = $state('');
	let sending = $state(false);
	let activeConversationId = $state<string | null>(null);
	let selectedModel = $state('');
	let selectedVoice = $state('');
	let models = $state<{ id: string; label: string }[]>([{ id: '', label: t('japabot.model_default', 'Default model') }]);
	let voices = $state<{ id: string; label: string }[]>([{ id: '', label: t('japabot.voice_default', 'Default voice') }]);

	const seo = $derived(
		buildSeoMeta({
			title: t('japabot.seo_title', 'JapaBot — Your AI Japa Guide | JapaConnect Tools'),
			description: t(
				'japabot.seo_description',
				'Chat with JapaBot — answers grounded in JapaConnect\'s published guides, forum threads, and community knowledge via RAG.'
			),
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
			{
				message: messageText,
				conversation_id: activeConversationId ?? undefined,
				model: selectedModel || undefined,
				voice_id: selectedVoice || undefined
			}
		);
		sending = false;

		if (res.error) {
			const errorMessage: ChatMessage = {
				id: crypto.randomUUID(),
				role: 'assistant',
				content: t('japabot.error_msg', "Sorry, I couldn't process your request. {message}").replace('{message}', res.error.message),
				created_at: new Date().toISOString()
			};
			messages = [...messages, errorMessage];
			return;
		}

		if (res.data) {
			const wasNewConversation = !activeConversationId;
			activeConversationId = res.data.conversation_id;
			const assistantMessage: ChatMessage = {
				id: crypto.randomUUID(),
				role: 'assistant',
				content: res.data.reply,
				created_at: new Date().toISOString(),
				sources: res.data.sources
			};
			messages = [...messages, assistantMessage];
			// A brand-new conversation won't appear in the SSR-loaded sidebar
			// list — refresh so it shows up with a preview.
			if (wasNewConversation) refreshConversations();
		}
	}

	async function refreshConversations() {
		const res = await api.get<ChatConversationSummary[]>('/ai/conversations');
		if (res.data) conversations = res.data;
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

	/**
	 * Load a past conversation. The list endpoint only returns summaries
	 * (BUG-005), so the full turn history is fetched from
	 * GET /ai/conversations/{id} and its Honcho entries mapped to messages.
	 */
	async function loadConversation(conv: ChatConversationSummary) {
		if (loadingConversation) return;
		loadingConversation = true;
		activeConversationId = conv.conversation_id;
		const res = await api.get<ChatConversationDetail>(
			`/ai/conversations/${conv.conversation_id}`
		);
		loadingConversation = false;
		if (!res.data) return;
		messages = (res.data.entries ?? [])
			.filter((e) => e.metadata?.role === 'user' || e.metadata?.role === 'assistant')
			.map((e) => ({
				id: e.id,
				role: e.metadata?.role === 'assistant' ? ('assistant' as const) : ('user' as const),
				content: e.content,
				created_at: e.created_at
			}));
	}

	// Auto-scroll to bottom on new messages. Track messages.length + sending so
	// the effect re-runs whenever a message arrives or the typing indicator
	// appears (the container ref alone doesn't create a dependency).
	let messagesContainer: HTMLDivElement | null = $state(null);
	$effect(() => {
		messages.length;
		sending;
		if (messagesContainer) {
			messagesContainer.scrollTop = messagesContainer.scrollHeight;
		}
	});

	// $derived so the suggestions re-render when the locale changes (the t()
	// calls read i18n.locale, a $derived). A plain const would freeze the strings
	// at first render in whatever locale was active then.
	const suggestions = $derived([
		t('japabot.suggestion_1', 'What documents do I need for a UK student visa?'),
		t('japabot.suggestion_2', 'How do I find scholarships for masters?'),
		t('japabot.suggestion_3', "What's the cost of living in Toronto vs Lagos?"),
		t('japabot.suggestion_4', 'How do I send money home cheaply?')
	]);
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
	{#if !data.enabled}
		<ToolDisabled tool={data.tool} />
	{:else if !user}
		<!-- Sign-in required -->
		<div class="flex-1 flex items-center justify-center p-4">
			<div class="text-center max-w-sm">
				<div class="flex justify-center mb-4">
					<div class="w-16 h-16 rounded-2xl bg-[var(--tool-accent)] flex items-center justify-center">
						<Icon name="bot" size={32} class="text-white" />
					</div>
				</div>
				<h2 class="text-xl font-semibold text-text">{t('japabot.title', 'JapaBot')}</h2>
				<p class="mt-2 text-sm text-text-subtle">
					{t('japabot.subtitle', "Your AI guide for the japa journey. Ask about visas, scholarships, cost of living, and more — answers are grounded in JapaConnect's published content.")}
				</p>
				<div class="mt-5 flex flex-col gap-2">
					<a
						href={mainAppUrl('/login')}
						class="px-4 py-2.5 bg-[var(--tool-accent)] text-white rounded-lg text-sm font-medium hover:bg-[var(--tool-accent-hover)] transition-colors"
					>
						{t('japabot.sign_in_to_chat', 'Sign in to chat')}
					</a>
					<a
						href={mainAppUrl('/signup')}
						class="px-4 py-2.5 text-sm font-medium text-text hover:text-[var(--tool-accent)] transition-colors"
					>
						{t('common.create_account', 'Create free account')}
					</a>
				</div>
			</div>
		</div>
	{:else}
		<div class="flex flex-1 overflow-hidden">
			<!-- Conversation sidebar (desktop) — always visible so the
			     New chat affordance exists even with no prior conversations. -->
			<aside class="hidden md:flex flex-col w-64 border-r border-border bg-surface">
				<div class="p-3 border-b border-border">
					<button
						onclick={newConversation}
						class="w-full inline-flex items-center justify-center gap-2 px-3 py-2 bg-[var(--tool-accent)] text-white rounded-lg text-sm font-medium hover:bg-[var(--tool-accent-hover)] transition-colors"
					>
						<Icon name="plus" size={16} />
						{t('japabot.new_chat', 'New chat')}
					</button>
				</div>
				<div class="flex-1 overflow-y-auto p-2 space-y-1">
					{#each conversations as conv (conv.conversation_id)}
						<button
							onclick={() => loadConversation(conv)}
							class="w-full text-left px-3 py-2 text-sm rounded-lg truncate transition-colors {activeConversationId === conv.conversation_id ? 'bg-surface-subtle text-text' : 'text-text-subtle hover:bg-surface-subtle'}"
						>
							{conv.preview || t('japabot.new_conversation', 'New conversation')}
						</button>
					{/each}
				</div>
			</aside>

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
								<h2 class="text-lg font-semibold text-text">{t('japabot.ask_anything', 'Ask JapaBot anything')}</h2>
								<p class="mt-1 text-sm text-text-subtle max-w-md mx-auto">
									{t('japabot.ask_desc', "Answers are grounded in JapaConnect's published guides, forum threads, and community knowledge.")}
								</p>
								<div class="mt-6 grid grid-cols-1 sm:grid-cols-2 gap-2 max-w-md mx-auto">
									{#each suggestions as suggestion (suggestion)}
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
												<span class="text-xs text-muted">{t('japabot.sources', 'Sources:')}</span>
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
					<div class="max-w-2xl mx-auto flex items-end gap-2 mb-2">
						<div class="flex-1">
							<label for="japabot-model" class="sr-only">{t('japabot.model_label', 'Model')}</label>
							<select id="japabot-model" bind:value={selectedModel} class="w-full px-3 py-1.5 text-sm bg-surface border border-border rounded-lg text-text">
								{#each models as option (option.id)}
									<option value={option.id}>{option.label}</option>
								{/each}
							</select>
						</div>
						<div class="flex-1">
							<label for="japabot-voice" class="sr-only">{t('japabot.voice_label', 'Voice / Tone')}</label>
							<select id="japabot-voice" bind:value={selectedVoice} class="w-full px-3 py-1.5 text-sm bg-surface border border-border rounded-lg text-text">
								{#each voices as option (option.id)}
									<option value={option.id}>{option.label}</option>
								{/each}
							</select>
						</div>
					</div>
					<div class="max-w-2xl mx-auto flex items-end gap-2">
						<textarea
							bind:value={input}
							onkeydown={onKeydown}
							placeholder={t('japabot.input_placeholder', 'Ask about visas, scholarships, cost of living...')}
							rows="1"
							class="flex-1 px-4 py-3 bg-surface border border-border rounded-lg text-text resize-none focus:border-[var(--tool-accent)] focus:outline-none"
							style="min-height: 48px; max-height: 120px;"
						></textarea>
						<button
							onclick={sendMessage}
							disabled={!input.trim() || sending}
							class="flex-shrink-0 w-12 h-12 flex items-center justify-center bg-[var(--tool-accent)] text-white rounded-lg hover:bg-[var(--tool-accent-hover)] transition-colors disabled:opacity-50"
							aria-label={t('japabot.send', 'Send message')}
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

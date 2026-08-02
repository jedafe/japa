<script lang="ts">
	import { t } from '$lib/i18n';
	import { Send, Sparkles, MessageSquare, Loader2, AlertCircle } from 'lucide-svelte';
	import { enhance } from '$app/forms';

	let { data } = $props();

	type ChatMessage = {
		role: 'user' | 'assistant';
		content: string;
		conversation_id?: string;
	};

	// Seed the visible transcript with prior conversation entries (best-effort:
	// Honcho entries are flat context rows, so we render them as assistant
	// memory unless they carry an explicit role).
	let messages = $state<ChatMessage[]>(
		(data.conversations ?? []).map((c: { content?: string; role?: string; id?: string }) => ({
			role: (c.role as 'user' | 'assistant') ?? 'assistant',
			content: c.content ?? '',
			conversation_id: c.id
		}))
	);
	let input = $state('');
	let conversationId = $state<string>('');
	let sending = $state(false);
	let error = $state('');
	let scrollEl: HTMLElement | undefined = $state();

	// Auto-scroll to the latest message whenever the transcript grows.
	$effect(() => {
		void messages.length;
		if (scrollEl) {
			scrollEl.scrollTop = scrollEl.scrollHeight;
		}
	});
</script>

<svelte:head>
	<title>{t('ai.chat.title') ?? 'JapaBot Chat'} — JapaConnect</title>
</svelte:head>

<div class="mx-auto flex h-[calc(100vh-8rem)] max-w-full flex-col">
	<header class="mb-4 flex items-center gap-3">
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
		class="flex-1 overflow-y-auto rounded-lg border p-4"
		style="border-color: var(--color-border); background-color: var(--color-surface)"
	>
		{#if messages.length === 0}
			<div class="flex h-full flex-col items-center justify-center gap-2 text-center">
				<MessageSquare class="h-8 w-8" style="color: var(--color-muted)" />
				<p class="text-sm" style="color: var(--color-muted)">{t('ai.chat.empty') ?? 'Start a conversation with JapaBot'}</p>
			</div>
		{:else}
			<div class="space-y-3">
				{#each messages as m}
					<div class="flex {m.role === 'user' ? 'justify-end' : 'justify-start'}">
						<div
							class="max-w-[80%] rounded-2xl px-4 py-2 text-sm whitespace-pre-wrap"
							style={m.role === 'user'
								? 'background-color: var(--color-primary); color: white'
								: 'background-color: var(--color-muted-15); color: var(--color-text)'}
						>
							{m.content}
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
					const data = result.data as { reply: string; conversation_id?: string };
					messages = [...messages, { role: 'assistant', content: data.reply, conversation_id: data.conversation_id }];
					if (data.conversation_id) conversationId = data.conversation_id;
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

<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { page } from '$app/state';
	import { ArrowLeft, Send, Loader2 } from 'lucide-svelte';
	import { api, type DMMessage, type DMConversation } from '$lib/api';
	import { isFeatureEnabled } from '$lib/stores/features.svelte';
	import { formatTimestampWithTime } from '$lib/utils/timezone';
	import { onMount, tick } from 'svelte';
	import { wsClient } from '$lib/websocket';
	import TypingIndicator from '$lib/components/TypingIndicator.svelte';
	import autoAnimate from '@formkit/auto-animate';

	let messages = $state<DMMessage[]>([]);
	let loading = $state(true);
	let loadingMore = $state(false);
	let hasMore = $state(true);
	let newMessage = $state('');
	let sending = $state(false);
	let error = $state('');
	let messageContainer = $state<HTMLDivElement | null>(null);
	let messagesList: HTMLDivElement | undefined = $state();
	let otherUserName = $state('');
	let typingUser = $state<string | null>(null);
	let typingTimeout: ReturnType<typeof setTimeout> | null = null;
	let typingSendTimer: ReturnType<typeof setTimeout> | null = null;
	let isTyping = $state(false);

	const channelId = $derived(page.params.channelId);

	async function loadMessages(initial = true) {
		if (!channelId) return;
		if (initial) {
			loading = true;
		} else {
			loadingMore = true;
		}
		error = '';
		try {
			const result = await api.getDMMessages(channelId, { page_size: 50 });
			if (initial) {
				messages = result;
				// Get channel info for the other user's name
				try {
					const channels = await api.listDMConversations();
					const ch = channels.find((c) => c.id === channelId);
					if (ch) otherUserName = ch.other_display_name || ch.other_username;
				} catch {
					otherUserName = 'User';
				}
			} else {
				messages = [...result, ...messages];
			}
			hasMore = result.length === 50;
		} catch {
			error = 'Failed to load messages.';
		} finally {
			loading = false;
			loadingMore = false;
			if (initial) {
				await tick();
				scrollToBottom();
			}
		}
	}

	function scrollToBottom() {
		if (messageContainer) {
			messageContainer.scrollTop = messageContainer.scrollHeight;
		}
	}

	async function handleSend(e: Event) {
		e.preventDefault();
		if (!channelId || !newMessage.trim()) return;
		sending = true;
		try {
			await api.sendDMMessage(channelId, { body: newMessage.trim() });
			// Reload messages to get the new one with proper sender info
			await loadMessages();
			newMessage = '';
			stopTyping();
		} catch {
			error = 'Failed to send message.';
		} finally {
			sending = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			handleSend(e);
		}
	}

	function handleInput() {
		// Send typing event (debounced)
		if (!typingSendTimer) {
			wsClient.send('typing', { conversation_id: channelId });
			isTyping = true;
		}

		// Clear existing timer
		if (typingSendTimer) clearTimeout(typingSendTimer);

		// Stop sending typing events after 3 seconds of no input
		typingSendTimer = setTimeout(() => {
			isTyping = false;
			typingSendTimer = null;
		}, 3000);
	}

	function stopTyping() {
		isTyping = false;
		if (typingSendTimer) {
			clearTimeout(typingSendTimer);
			typingSendTimer = null;
		}
	}

	function formatTime(dateStr: string): string {
		return new Date(dateStr).toLocaleTimeString('en-US', {
			hour: '2-digit',
			minute: '2-digit'
		});
	}

	function formatDate(dateStr: string): string {
		return formatTimestampWithTime(dateStr);
	}

	onMount(() => {
		loadMessages();
	});

	$effect(() => {
		if (!channelId) return;
		// Reset state when channel changes
		messages = [];
		typingUser = null;
		loadMessages();
	});

	$effect(() => {
		if (messagesList) {
			autoAnimate(messagesList);
		}
	});

	// Listen for incoming DM messages
	$effect(() => {
		if (!channelId) return;
		const unsub = wsClient.on('dm_message', (message: { conversation_id?: string; id?: string; sender_id?: string; sender_display_name?: string; sender_username?: string; body?: string; created_at?: string }) => {
			if (message.conversation_id !== channelId) return;
			// Don't add if we already have this message
			const exists = messages.some((m) => m.id === message.id);
			if (exists || !message.id) return;

			messages = [...messages, {
				id: message.id,
				sender_id: message.sender_id ?? '',
				sender_username: message.sender_username ?? '',
				sender_display_name: message.sender_display_name ?? '',
				body: message.body ?? '',
				created_at: message.created_at ?? new Date().toISOString(),
				read_at: undefined
			}];
			tick().then(() => scrollToBottom());
		});
		return unsub;
	});

	// Listen for typing events
	$effect(() => {
		if (!channelId) return;
		const unsub = wsClient.on('typing', (data: { conversation_id?: string; username?: string }) => {
			if (data.conversation_id !== channelId) return;
			typingUser = data.username ?? otherUserName;

			// Clear existing timeout
			if (typingTimeout) clearTimeout(typingTimeout);

			// Hide typing indicator after 3 seconds
			typingTimeout = setTimeout(() => {
				typingUser = null;
			}, 3000);
		});
		return unsub;
	});
</script>

<svelte:head>
	<title>{otherUserName || 'Chat'} | Messages | JapaConnect</title>
</svelte:head>

{#if !isFeatureEnabled('feature.dm')}
	<main class="mx-auto max-w-2xl px-6 py-16">
		<div class="text-center">
			<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">{t('messages.channelId.messages_is_not_available')}</h1>
			<p class="mt-2" style="color: var(--color-muted)">{t('messages.channelId.this_feature_is_currently_disabled')}</p>
			<a href="/" class="mt-4 inline-block" style="color: var(--color-primary)">{t('messages.channelId.go_home')}</a>
		</div>
	</main>
{:else}
	<div class="flex h-screen flex-col">
	<!-- Header -->
	<header
		class="flex items-center gap-3 border-b px-4 py-3"
		style="border-color: var(--color-border); background-color: var(--color-surface)"
	>
		<a
			href="/messages"
			class="rounded-lg p-1.5 transition-colors hover:bg-black/10 dark:hover:bg-white/10"
			style="color: var(--color-muted)"
		>
			<ArrowLeft class="h-5 w-5" />
		</a>
		<div class="flex-1 min-w-0">
			<h1 class="text-sm font-semibold truncate" style="color: var(--color-foreground)">
				{otherUserName || 'Chat'}
			</h1>
		</div>
	</header>

	<!-- Messages -->
	<div
		bind:this={messageContainer}
		class="flex-1 overflow-y-auto px-4 py-4"
		style="background-color: var(--color-background)"
	>
		{#if loading}
			<div class="flex items-center justify-center py-12">
				<Loader2 class="h-6 w-6 animate-spin" style="color: var(--color-muted)" />
			</div>
		{:else if error}
			<div class="py-12 text-center">
				<p class="text-sm" style="color: var(--color-danger)">{error}</p>
			</div>
		{:else if messages.length === 0}
			<div class="py-12 text-center">
				<p class="text-sm" style="color: var(--color-muted)">{t('messages.channelId.no_messages_yet_say_hello')}</p>
			</div>
		{:else}
			{#if hasMore}
				<button
					onclick={() => loadMessages(false)}
					disabled={loadingMore}
					class="mx-auto mb-4 block rounded-lg px-3 py-1.5 text-xs font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
					style="color: var(--color-muted)"
				>
					{#if loadingMore}
						<Loader2 class="inline h-3 w-3 animate-spin" />
						Loading...
					{:else}
						Load older messages
					{/if}
				</button>
			{/if}

			<div bind:this={messagesList} class="space-y-3">
				{#each messages as msg (msg.id)}
					<div class="flex justify-start">
						<div
							class="max-w-[75%] rounded-2xl px-4 py-2.5"
							style="background-color: var(--color-surface); color: var(--color-foreground)"
						>
							<p class="text-xs font-medium" style="color: var(--color-muted)">{msg.sender_display_name || msg.sender_username}</p>
							<p class="text-sm leading-relaxed">{msg.body}</p>
							<p class="mt-1 text-[10px]" style="color: var(--color-muted)">
								{formatTime(msg.created_at)}
							</p>
						</div>
					</div>
				{/each}
			</div>
		{/if}

		<!-- Typing indicator -->
		{#if typingUser}
			<div class="mt-3">
				<TypingIndicator username={typingUser} />
			</div>
		{/if}
	</div>

	<!-- Input -->
	<div
		class="border-t px-4 py-3"
		style="border-color: var(--color-border); background-color: var(--color-surface)"
	>
		<form onsubmit={handleSend} class="flex items-end gap-3">
			<textarea
				placeholder="Type a message..."
				bind:value={newMessage}
				onkeydown={handleKeydown}
				oninput={handleInput}
				rows="1"
				class="flex-1 resize-none rounded-xl border px-4 py-2.5 text-sm outline-none transition-colors focus:ring-2"
				style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
			></textarea>
			<button
				type="submit"
				disabled={sending || !newMessage.trim()}
				class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl text-white transition-opacity hover:opacity-90 disabled:opacity-50"
				style="background-color: var(--color-primary)"
			>
				{#if sending}
					<Loader2 class="h-4 w-4 animate-spin" />
				{:else}
					<Send class="h-4 w-4" />
				{/if}
			</button>
		</form>
	</div>
	</div>
{/if}

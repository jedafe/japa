<script lang="ts">
	import { onMount } from 'svelte';
	import { request } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Send } from 'lucide-svelte';

	type Message = {
		id: string;
		role: string;
		content: string;
		created_at: string;
	};

	let messages = $state<Message[]>([]);
	let input = $state('');
	let loading = $state(true);
	let sending = $state(false);
	let errorMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			messages = await request<Message[]>('/ai/conversations');
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function send() {
		if (!input.trim()) return;
		sending = true;
		errorMessage = '';
		try {
			await request('/ai/chat', { method: 'POST', body: JSON.stringify({ message: input.trim() }) });
			input = '';
			load();
		} catch {
			errorMessage = t('common.error');
		} finally {
			sending = false;
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('ai.chat.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('ai.chat.title')}</h1>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="flex h-[60vh] flex-col rounded-lg border" style="border-color: var(--color-border)">
		<div class="flex-1 overflow-y-auto p-4 space-y-3">
			{#each messages as msg (msg.id)}
				<div class="flex {msg.role === 'user' ? 'justify-end' : 'justify-start'}">
					<div class="max-w-[80%] rounded-lg px-4 py-2 text-sm" style="background: {msg.role === 'user' ? 'var(--color-primary)' : 'var(--color-surface)'}; color: {msg.role === 'user' ? 'white' : 'var(--color-foreground)'}">
						{msg.content}
					</div>
				</div>
			{:else}
				<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('ai.chat.empty')}</p>
			{/each}
		</div>
		<div class="border-t p-4" style="border-color: var(--color-border)">
			<div class="flex gap-2">
				<input type="text" bind:value={input} onkeydown={(e) => e.key === 'Enter' && send()} placeholder={t('ai.chat.placeholder')} class="flex-1 rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)" />
				<button class="btn btn-primary" onclick={send} disabled={sending || !input.trim()}>
					<Send class="h-4 w-4" />
				</button>
			</div>
		</div>
	</div>
{/if}

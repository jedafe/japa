<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { EmptyState } from '$lib/components/ui';
	import { UserX } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let requests = $state<PageData['requests']>(untrack(() => data.requests));
	let acting = $state<string | null>(null);

	$effect(() => {
		requests = data.requests;
	});

	const title = $derived(t('messages.requests_title', 'Message requests — JapaConnect'));

	async function handleAction(
		id: string,
		conversationId: string | undefined,
		action: 'accept' | 'decline' | 'block'
	) {
		acting = id;
		const res = await api.post(`/dm/requests/${id}/${action}`);
		acting = null;
		if (res.error) return;

		// Remove the request from the list after action.
		requests = requests.filter((r) => r.id !== id);

		// If accepted, navigate to the conversation.
		if (action === 'accept' && conversationId) {
			await goto(`/messages?c=${conversationId}`);
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

<div class="mx-auto w-full max-w-2xl">
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="text-base font-semibold text-[var(--color-text)]">
			{t('messages.requests_heading', 'Message requests')}
		</h1>
	</div>

	{#if requests.length === 0}
		<div class="p-4">
			<EmptyState icon={UserX} title={t('messages.no_requests', 'No pending requests')} />
		</div>
	{:else}
		<ul class="divide-y divide-[var(--color-border)]">
			{#each requests as req (req.id)}
				<li class="flex items-center gap-3 p-4">
					{#if req.requester_avatar_url}
						<img
							src={req.requester_avatar_url}
							alt=""
							class="h-10 w-10 shrink-0 rounded-full object-cover"
						/>
					{:else}
						<span
							class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]"
						>
							{req.requester_display_name.charAt(0).toUpperCase()}
						</span>
					{/if}
					<div class="min-w-0 flex-1">
						<p class="truncate text-sm font-medium text-[var(--color-text)]">
							{req.requester_display_name}
						</p>
						<p class="truncate text-xs text-[var(--color-muted)]">
							@{req.requester_username}
						</p>
						<p class="mt-1 truncate text-sm text-[var(--color-text-subtle)]">
							{req.preview_message}
						</p>
					</div>
					<div class="flex shrink-0 gap-2">
						<button
							type="button"
							onclick={() => handleAction(req.id, req.conversation_id, 'accept')}
							disabled={acting === req.id}
							class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
						>
							{t('messages.requests_accept', 'Accept')}
						</button>
						<button
							type="button"
							onclick={() => handleAction(req.id, undefined, 'decline')}
							disabled={acting === req.id}
							class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
						>
							{t('messages.requests_decline', 'Decline')}
						</button>
						<button
							type="button"
							onclick={() => handleAction(req.id, undefined, 'block')}
							disabled={acting === req.id}
							class="rounded-[var(--radius)] px-3 py-1.5 text-xs font-medium text-[var(--color-danger)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
						>
							{t('messages.requests_block', 'Block')}
						</button>
					</div>
				</li>
			{/each}
		</ul>
	{/if}
</div>

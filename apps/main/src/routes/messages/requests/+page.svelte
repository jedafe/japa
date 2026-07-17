<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { goto } from '$app/navigation';
	import { ArrowLeft, Check, X, Ban, MessageCircle } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api, type DMConversation } from '$lib/api';
	import { isFeatureEnabled } from '$lib/stores/features.svelte';
	import { onMount } from 'svelte';

	let requests = $state<DMConversation[]>([]);
	let loading = $state(true);

	onMount(async () => {
		try {
			requests = await api.listDMRequests();
		} catch {
			requests = [];
		} finally {
			loading = false;
		}
	});

	async function acceptRequest(id: string) {
		try {
			await api.acceptDMRequest(id);
			requests = requests.filter(r => r.id !== id);
		} catch {
			// silently fail
		}
	}

	async function declineRequest(id: string) {
		try {
			await api.declineDMRequest(id);
			requests = requests.filter(r => r.id !== id);
		} catch {
			// silently fail
		}
	}

	async function blockUser(id: string) {
		if (!confirm('Block this user?')) return;
		try {
			await api.blockDMUser(id);
			requests = requests.filter(r => r.id !== id);
		} catch {
			// silently fail
		}
	}

	function timeAgo(dateStr: string): string {
		const diff = Date.now() - new Date(dateStr).getTime();
		const mins = Math.floor(diff / 60000);
		if (mins < 60) return `${mins}m ago`;
		const hours = Math.floor(mins / 60);
		if (hours < 24) return `${hours}h ago`;
		const days = Math.floor(hours / 24);
		return `${days}d ago`;
	}
</script>

<svelte:head>
	<title>Message Requests | JapaConnect</title>
</svelte:head>

{#if !isFeatureEnabled('feature.dm')}
	<main class="mx-auto max-w-2xl px-6 py-16 text-center">
		<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">{t('messages.requests.messages_not_available')}</h1>
		<p class="mt-2" style="color: var(--color-muted)">{t('messages.requests.this_feature_is_currently_disabled')}</p>
		<a href="/" class="mt-4 inline-block" style="color: var(--color-primary)">{t('messages.requests.go_home')}</a>
	</main>
{:else}
	<main class="mx-auto max-w-2xl px-4 py-8">
		<a
			href="/messages"
			class="mb-6 inline-flex items-center gap-1.5 text-sm font-medium hover:underline"
			style="color: var(--color-muted)"
		>
			<ArrowLeft class="h-4 w-4" />
			Back to messages
		</a>

		<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">{t('messages.requests.message_requests')}</h1>
		<p class="mt-2 text-sm" style="color: var(--color-muted)">
			People who want to message you
		</p>

		{#if loading}
			<div class="mt-6 space-y-3">
				{#each Array(3) as _}
					<div class="h-20 animate-pulse rounded-xl" style="background-color: var(--color-surface)"></div>
				{/each}
			</div>
		{:else if requests.length === 0}
			<div class="mt-12">
				<EmptyState
					icon={MessageCircle}
					title="No pending requests"
					message="You don't have any pending message requests."
				/>
			</div>
		{:else}
			<div class="mt-6 space-y-3">
				{#each requests as req (req.id)}
					<div
						class="flex items-center gap-4 rounded-xl border p-4"
						style="border-color: var(--color-border); background: var(--color-surface)"
					>
						<div
							class="flex h-11 w-11 shrink-0 items-center justify-center rounded-full text-sm font-semibold"
							style="background-color: color-mix(in srgb, var(--color-primary) 12%, transparent); color: var(--color-primary)"
						>
							{req.other_display_name?.charAt(0).toUpperCase() ?? req.other_username.charAt(0).toUpperCase()}
						</div>
						<div class="flex-1 min-w-0">
							<h3 class="text-sm font-semibold" style="color: var(--color-foreground)">
								{req.other_display_name || req.other_username}
							</h3>
							<p class="text-xs" style="color: var(--color-muted)">
								{timeAgo(req.last_message_at ?? req.created_at)}
							</p>
						</div>
						<div class="flex items-center gap-2">
							<button
								onclick={() => acceptRequest(req.id)}
								class="rounded-lg p-2 text-white transition-opacity hover:opacity-80"
								style="background: var(--color-primary)"
								title="Accept"
							>
								<Check class="h-4 w-4" />
							</button>
							<button
								onclick={() => declineRequest(req.id)}
								class="rounded-lg border p-2 transition-colors hover:bg-gray-100 dark:hover:bg-gray-700"
								style="border-color: var(--color-border)"
								title="Decline"
							>
								<X class="h-4 w-4" style="color: var(--color-muted)" />
							</button>
							<button
								onclick={() => blockUser(req.id)}
								class="rounded-lg border p-2 transition-colors hover:bg-red-50 dark:hover:bg-red-900/20"
								style="border-color: var(--color-border)"
								title="Block"
							>
								<Ban class="h-4 w-4" style="color: #ef4444" />
							</button>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</main>
{/if}

<script lang="ts">
	import { MessageCircle, Plus, Loader2 } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api, type DMConversation } from '$lib/api';
	import { isFeatureEnabled } from '$lib/stores/features.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { onMount } from 'svelte';

	let channels = $state<DMConversation[]>([]);
	let loading = $state(true);

	function timeAgo(dateStr: string): string {
		const diff = Date.now() - new Date(dateStr).getTime();
		const mins = Math.floor(diff / 60000);
		if (mins < 60) return `${mins}m ago`;
		const hours = Math.floor(mins / 60);
		if (hours < 24) return `${hours}h ago`;
		const days = Math.floor(hours / 24);
		return `${days}d ago`;
	}

	onMount(async () => {
		try {
			channels = await api.listDMConversations();
		} catch {
			channels = [];
		} finally {
			loading = false;
		}
	});
</script>

<svelte:head>
	<title>Messages | JapaConnect</title>
	<meta name="description" content="Your direct messages." />
</svelte:head>

{#if !isFeatureEnabled('feature.dm')}
	<main class="mx-auto max-w-2xl px-6 py-16">
		<div class="text-center">
			<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">{t('messages.not_available')}</h1>
			<p class="mt-2" style="color: var(--color-muted)">{t('forum.feature_disabled')}</p>
			<a href="/" class="mt-4 inline-block" style="color: var(--color-primary)">{t('forum.go_home')}</a>
		</div>
	</main>
{:else}
	<main class="mx-auto max-w-2xl px-4 py-8">
	<header class="mb-8 flex items-start justify-between">
		<div>
			<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">{t('messages.title')}</h1>
			<p class="mt-2 text-sm" style="color: var(--color-muted)">
				{t('messages.subtitle')}
			</p>
		</div>
	</header>

	{#if loading}
		<div class="space-y-3">
			{#each Array(5) as _}
				<div class="h-20 animate-pulse rounded-xl" style="background-color: var(--color-surface)"></div>
			{/each}
		</div>
	{:else if channels.length === 0}
		<EmptyState
			icon={MessageCircle}
			title={t('messages.no_conversations')}
			message={t('messages.no_conversations_msg')}
		/>
	{:else}
		<div class="divide-y rounded-xl border" style="border-color: var(--color-border); divide-color: var(--color-border)">
			{#each channels as channel (channel.id)}
				<a
					href="/messages/{channel.id}"
					class="flex items-center gap-4 p-4 transition-colors hover:bg-black/5 dark:hover:bg-white/5"
				>
				<!-- Avatar -->
				<div
					class="flex h-11 w-11 shrink-0 items-center justify-center rounded-full text-sm font-semibold"
					style="background-color: color-mix(in srgb, var(--color-primary) 12%, transparent); color: var(--color-primary)"
				>
					{channel.other_display_name?.charAt(0).toUpperCase() ?? channel.other_username.charAt(0).toUpperCase()}
				</div>

				<!-- Content -->
				<div class="flex-1 min-w-0">
					<div class="flex items-center justify-between gap-2">
						<h3
							class="text-sm font-semibold truncate"
							style="color: {channel.unread_count > 0 ? 'var(--color-foreground)' : 'var(--color-muted)'}"
						>
							{channel.other_display_name || channel.other_username}
						</h3>
						{#if channel.last_message_at}
							<span class="text-[11px] shrink-0" style="color: var(--color-muted)">
								{timeAgo(channel.last_message_at)}
							</span>
						{/if}
					</div>
					<div class="mt-0.5 flex items-center gap-2">
						<p class="text-sm truncate" style="color: var(--color-muted)">
							{channel.last_message_preview ?? t('messages.no_messages')}
						</p>
							{#if channel.unread_count > 0}
								<span
									class="inline-flex h-5 min-w-5 shrink-0 items-center justify-center rounded-full px-1.5 text-[10px] font-bold text-white"
									style="background-color: var(--color-primary)"
								>
									{channel.unread_count}
								</span>
							{/if}
						</div>
					</div>
				</a>
			{/each}
		</div>
	{/if}
	</main>
{/if}

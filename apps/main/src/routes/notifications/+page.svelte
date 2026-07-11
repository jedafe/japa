<script lang="ts">
	import { Bell, Check, CheckCheck, Loader2 } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api } from '$lib/api';
	import autoAnimate from '@formkit/auto-animate';
	import { t } from '$lib/stores/i18n.svelte';
	import { wsClient } from '$lib/websocket';
	import { realtime } from '$lib/stores/realtime.svelte';

	interface Notification {
		id: string;
		title: string;
		body: string;
		read: boolean;
		created_at: string;
	}

	let { data } = $props();

	let notifications = $derived(data.notifications);
	let markingRead = $state<string | null>(null);
	let errorMessage = $state('');
	let listContainer: HTMLDivElement | undefined = $state();

	const unreadCount = $derived(notifications.filter((n) => !n.read).length);

	$effect(() => {
		if (!listContainer) return;
		autoAnimate(listContainer);
	});

	// Listen for real-time notifications
	$effect(() => {
		const unsub = wsClient.on('notification', (notification: Notification) => {
			// Prepend new notification if not already in list
			const exists = notifications.some((n) => n.id === notification.id);
			if (!exists && notification.id) {
				notifications = [notification, ...notifications];
			}
		});
		return unsub;
	});

	async function markAsRead(id: string) {
		markingRead = id;
		errorMessage = '';
		try {
			await api.markNotificationRead(id);
			notifications = notifications.map((n) => (n.id === id ? { ...n, read: true } : n));
			// Update realtime count
			const newUnread = notifications.filter((n) => !n.read).length;
			realtime.setNotificationCount(newUnread);
		} catch {
			errorMessage = 'Failed to mark notification as read. Please try again.';
		} finally {
			markingRead = null;
		}
	}

	async function markAllAsRead() {
		try {
			await api.markAllRead();
			notifications = notifications.map((n) => ({ ...n, read: true }));
			realtime.setNotificationCount(0);
		} catch {
			// ignore
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
	<title>{t('notifications.title')} | JapaConnect</title>
	<meta name="description" content="View your notifications." />
</svelte:head>

<main class="mx-auto max-w-2xl px-4 py-8">
	<header class="mb-6 flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">
				{t('notifications.title')}
			</h1>
			{#if unreadCount > 0}
				<p class="mt-1 text-sm" style="color: var(--color-muted)">
					{t('notifications.unread').replace('{count}', String(unreadCount))}
				</p>
			{/if}
		</div>
		{#if unreadCount > 0}
			<button
				onclick={markAllAsRead}
				class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
				style="color: var(--color-primary)"
			>
				<CheckCheck class="h-4 w-4" />
				{t('notifications.mark_all_read')}
			</button>
		{/if}
	</header>

	{#if notifications.length === 0}
		<EmptyState
			icon={Bell}
			title={t('notifications.empty_title')}
			message={t('notifications.empty_message')}
		/>
	{:else}
		{#if errorMessage}
			<div class="mb-4 rounded-lg border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
				{errorMessage}
			</div>
		{/if}
		<div
			bind:this={listContainer}
			class="divide-y rounded-xl border"
			style="border-color: var(--color-border); divide-color: var(--color-border)"
		>
			{#each notifications as notification (notification.id)}
				<div
					class="flex items-start gap-3 p-4 transition-colors hover:bg-black/5 dark:hover:bg-white/5"
					style="background-color: {notification.read ? 'transparent' : 'color-mix(in srgb, var(--color-primary) 5%, transparent)'}"
				>
					<div class="flex-1 min-w-0">
						<p class="text-sm font-medium" style="color: var(--color-foreground)">
							{notification.title}
						</p>
						<p class="mt-1 text-sm line-clamp-2" style="color: var(--color-muted)">
							{notification.body}
						</p>
						<p class="mt-1 text-xs" style="color: var(--color-muted)">
							{timeAgo(notification.created_at)}
						</p>
					</div>
					{#if !notification.read}
						<button
							onclick={() => markAsRead(notification.id)}
							disabled={markingRead === notification.id}
							class="shrink-0 rounded-md p-1.5 hover:bg-black/10 dark:hover:bg-white/10"
							style="color: var(--color-muted)"
							aria-label="Mark as read"
						>
							{#if markingRead === notification.id}
								<Loader2 class="h-4 w-4 animate-spin" />
							{:else}
								<Check class="h-4 w-4" />
							{/if}
						</button>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</main>

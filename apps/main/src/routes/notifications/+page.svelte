<script lang="ts">
	import { Bell, Check, CheckCheck, Loader2 } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { getToken } from '$lib/api';
	import { onMount } from 'svelte';

	interface Notification {
		id: string;
		title: string;
		body: string;
		read: boolean;
		created_at: string;
	}

	let notifications = $state<Notification[]>([]);
	let loading = $state(true);
	let markingRead = $state<string | null>(null);
	let errorMessage = $state('');

	const unreadCount = $derived(notifications.filter((n) => !n.read).length);

	function authHeaders(extra?: Record<string, string>): Record<string, string> {
		const h: Record<string, string> = { 'Content-Type': 'application/json', ...extra };
		const token = getToken();
		if (token) h['Authorization'] = `Bearer ${token}`;
		return h;
	}

	onMount(async () => {
		try {
			const res = await fetch('/api/v1/notifications?limit=50', { headers: authHeaders() });
			const body = await res.json();
			if (body.data) {
				notifications = body.data;
			}
		} catch {
			notifications = [];
		} finally {
			loading = false;
		}
	});

	async function markAsRead(id: string) {
		markingRead = id;
		errorMessage = '';
		try {
			await fetch(`/api/v1/notifications/${id}/read`, { method: 'PATCH', headers: authHeaders() });
			notifications = notifications.map((n) => (n.id === id ? { ...n, read: true } : n));
		} catch {
			errorMessage = 'Failed to mark notification as read. Please try again.';
		} finally {
			markingRead = null;
		}
	}

	async function markAllAsRead() {
		try {
			await fetch('/api/v1/notifications/read-all', { method: 'POST', headers: authHeaders() });
			notifications = notifications.map((n) => ({ ...n, read: true }));
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
	<title>Notifications | JapaConnect</title>
	<meta name="description" content="View your notifications." />
</svelte:head>

<main class="mx-auto max-w-2xl px-4 py-8">
	<header class="mb-6 flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">
				Notifications
			</h1>
			{#if unreadCount > 0}
				<p class="mt-1 text-sm" style="color: var(--color-muted)">
					{unreadCount} unread
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
				Mark all read
			</button>
		{/if}
	</header>

	{#if loading}
		<div class="space-y-3">
			{#each Array(5) as _}
				<div
					class="h-20 animate-pulse rounded-lg"
					style="background-color: var(--color-surface)"
				></div>
			{/each}
		</div>
	{:else if notifications.length === 0}
		<EmptyState
			icon={Bell}
			title="No notifications"
			message="You're all caught up! New notifications will appear here."
		/>
	{:else}
		{#if errorMessage}
			<div class="mb-4 rounded-lg border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
				{errorMessage}
			</div>
		{/if}
		<div class="divide-y rounded-xl border" style="border-color: var(--color-border); divide-color: var(--color-border)">
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

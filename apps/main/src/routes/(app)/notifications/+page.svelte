<script lang="ts">
	import { untrack, onMount } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Bell, CheckCheck, BellOff } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { EmptyState } from '$lib/components/ui';
	import { realtime } from '$lib/realtime/realtime.svelte';
	import type { Notification } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let notifications = $state<Notification[]>(untrack(() => data.notifications));
	let unreadCount = $state<number>(untrack(() => data.unreadCount));
	let markingAll = $state(false);

	// Re-seed on SSR payload change.
	$effect(() => {
		notifications = data.notifications;
		unreadCount = data.unreadCount;
		realtime.setUnreadCount(data.unreadCount);
	});

	const title = $derived(t('notifications.title', 'Notifications — JapaConnect'));

	// Phase 23 — live notification prepend via WebSocket. The realtime store
	// increments the global unread counter; this handler prepends the row so
	// the user sees it without a reload.
	onMount(() => {
		const handler = (n: Partial<Notification> & { id?: string }) => {
			if (!n.id) return;
			const row: Notification = {
				id: n.id,
				notification_type: n.notification_type ?? 'system',
				actor_type: n.actor_type ?? null,
				actor_id: n.actor_id ?? null,
				subject_type: n.subject_type ?? null,
				subject_id: n.subject_id ?? null,
				title: n.title ?? '',
				body: n.body ?? null,
				action_url: n.action_url ?? null,
				read_at: null,
				created_at: n.created_at ?? new Date().toISOString(),
				type_label: n.type_label ?? t('notifications.type_system', 'System')
			};
			notifications = [row, ...notifications];
			unreadCount += 1;
		};
		realtime.onNotification(handler);
		return () => realtime.offNotification(handler);
	});

	async function markRead(id: string) {
		const res = await api.post(`/me/notifications/${id}/read`);
		if (res.error) return;
		notifications = notifications.map((n) => (n.id === id ? { ...n, read_at: new Date().toISOString() } : n));
		unreadCount = Math.max(0, unreadCount - 1);
		realtime.decrementUnread();
	}

	async function markAllRead() {
		if (markingAll) return;
		markingAll = true;
		const res = await api.post('/me/notifications/read-all');
		markingAll = false;
		if (res.error) return;
		notifications = notifications.map((n) => ({ ...n, read_at: n.read_at ?? new Date().toISOString() }));
		unreadCount = 0;
		realtime.clearUnread();
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
	<div class="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Bell size={20} />
			{t('notifications.heading', 'Notifications')}
			{#if unreadCount > 0}
				<span class="rounded-full bg-[var(--color-primary)] px-2 py-0.5 text-xs font-medium text-[var(--color-primary-contrast)]">{unreadCount}</span>
			{/if}
		</h1>
		{#if notifications.length > 0 && unreadCount > 0}
			<button
				type="button"
				onclick={markAllRead}
				disabled={markingAll}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
			>
				<CheckCheck size={14} /> {t('notifications.mark_all_read', 'Mark all read')}
			</button>
		{/if}
	</div>

	<div class="p-3">
		{#if notifications.length === 0}
			<EmptyState icon={BellOff} title={t('notifications.empty', 'No notifications yet')} />
		{:else}
			<ul class="divide-y divide-[var(--color-border)] overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
				{#each notifications as n (n.id)}
					{@const unread = !n.read_at}
					<li class="relative {unread ? 'bg-[color-mix(in_srgb,var(--color-primary)_5%,transparent)]' : ''}">
						<a
							href={n.action_url ?? '#'}
							onclick={() => unread && markRead(n.id)}
							class="block p-4 transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							<div class="flex items-start gap-3">
								{#if unread}
									<span class="mt-1.5 h-2 w-2 shrink-0 rounded-full bg-[var(--color-primary)]" aria-label="unread"></span>
								{:else}
									<span class="mt-1.5 h-2 w-2 shrink-0 rounded-full bg-transparent"></span>
								{/if}
								<div class="min-w-0 flex-1">
									<p class="text-sm font-medium text-[var(--color-text)]">{n.title}</p>
									{#if n.body}
										<p class="mt-0.5 text-sm text-[var(--color-text-subtle)]">{n.body}</p>
									{/if}
									<p class="mt-1 text-xs text-[var(--color-muted)]">{n.type_label}</p>
								</div>
							</div>
						</a>
					</li>
				{/each}
			</ul>
		{/if}
	</div>
</div>

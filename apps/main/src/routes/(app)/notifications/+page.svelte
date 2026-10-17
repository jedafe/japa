<script lang="ts">
	import { untrack, onMount } from 'svelte';
	import { goto, replaceState } from '$app/navigation';
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { Bell, CheckCheck, Settings, Loader2, BellOff } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { EmptyState } from '$lib/components/ui';
	import { realtime } from '$lib/realtime/realtime.svelte';
	import NotificationItem from '$lib/components/notifications/NotificationItem.svelte';
	import {
		NOTIFICATION_FILTER_TABS,
		notificationFilterFor,
		type NotificationFilterTab
	} from '$lib/components/notifications/notificationType';
	import type { Notification } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let notifications = $state<Notification[]>(untrack(() => data.notifications));
	let unreadCount = $state<number>(untrack(() => data.unreadCount));
	let pageSize = $state<number>(untrack(() => data.pageSize));
	// Server-side offset tracking: `notifications` also holds live-prepended
	// WS rows, so notifications.length would over-count and silently skip
	// server rows. fetchedOffset counts only rows the API has served.
	let fetchedOffset = $state<number>(untrack(() => data.notifications.length));
	let loadingMore = $state(false);
	// Prefer the server's `total`/`has_more` sidecars when present; fall back
	// to the short-page heuristic for older backends without them.
	let hasMore = $state(
		untrack(() =>
			data.total != null
				? data.notifications.length < data.total
				: data.notifications.length === data.pageSize
		)
	);
	let markingAll = $state(false);

	// Re-seed on SSR payload change.
	$effect(() => {
		notifications = data.notifications;
		unreadCount = data.unreadCount;
		pageSize = data.pageSize;
		fetchedOffset = data.notifications.length;
		hasMore =
			data.total != null
				? data.notifications.length < data.total
				: data.notifications.length === data.pageSize;
		realtime.setUnreadCount(data.unreadCount);
	});

	// Active type filter, persisted in the URL query string.
	let activeFilter = $state<NotificationFilterTab>(
		untrack(() => {
			const param = page.url.searchParams.get('filter') as NotificationFilterTab;
			return NOTIFICATION_FILTER_TABS.some((tab) => tab.key === param) ? param : 'all';
		})
	);

	// Sync the active filter when the user navigates back/forward.
	$effect(() => {
		const param = page.url.searchParams.get('filter') as NotificationFilterTab;
		if (param && NOTIFICATION_FILTER_TABS.some((tab) => tab.key === param) && activeFilter !== param) {
			activeFilter = param;
		}
	});

	const filteredNotifications = $derived(
		activeFilter === 'all'
			? notifications
			: notifications.filter((n) => notificationFilterFor(n.notification_type) === activeFilter)
	);

	const title = $derived(t('notifications.title', 'Notifications — {site}'));

	// Phase 23 — live notification prepend via WebSocket. The realtime store
	// increments the global unread counter; this handler prepends the row so
	// the user sees it without a reload.
	onMount(() => {
		const handler = (n: Partial<Notification> & { id?: string }) => {
			if (!n.id) return;
			// Skip events for rows already shown (reconnect replay, double
			// delivery) so the keyed each-block never sees a duplicate key.
			if (notifications.some((x) => x.id === n.id)) return;
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

	function setFilter(filter: NotificationFilterTab) {
		if (activeFilter === filter) return;
		activeFilter = filter;
		const url = new URL(page.url);
		if (filter === 'all') {
			url.searchParams.delete('filter');
		} else {
			url.searchParams.set('filter', filter);
		}
		replaceState(url, {});
	}

	async function loadMore() {
		if (loadingMore || !hasMore) return;
		loadingMore = true;
		const res = await api.getEnvelope<Notification[]>(
			`/me/notifications?page_size=${pageSize}&page_offset=${fetchedOffset}`
		);
		loadingMore = false;
		if (res.error || !res.data) {
			hasMore = false;
			return;
		}
		// The offset window was consumed regardless of dedupe — advance it by
		// the number of rows the server returned, then drop rows already shown
		// (live prepends can overlap the window).
		fetchedOffset += res.data.length;
		if (res.data.length) {
			const seen = new Set(notifications.map((x) => x.id));
			const fresh = res.data.filter((x) => !seen.has(x.id));
			notifications = [...notifications, ...fresh];
		}
		const serverHasMore = res.envelope?.['has_more'];
		hasMore = typeof serverHasMore === 'boolean' ? serverHasMore : res.data.length >= pageSize;
	}

	/** Svelte action that triggers loadMore when the sentinel scrolls into view. */
	function infiniteScroll(node: HTMLElement) {
		const observer = new IntersectionObserver(
			(entries) => {
				if (entries[0]?.isIntersecting) {
					loadMore();
				}
			},
			{ threshold: 0, rootMargin: '100px' }
		);
		observer.observe(node);
		return {
			destroy() {
				observer.disconnect();
			}
		};
	}

	async function markRead(id: string) {
		const res = await api.post(`/me/notifications/${id}/read`);
		if (res.error) return;
		notifications = notifications.map((n) =>
			n.id === id ? { ...n, read_at: new Date().toISOString() } : n
		);
		unreadCount = Math.max(0, unreadCount - 1);
		realtime.decrementUnread();
	}

	async function markAllRead() {
		if (markingAll) return;
		markingAll = true;
		const res = await api.post('/me/notifications/read-all');
		markingAll = false;
		if (res.error) return;
		notifications = notifications.map((n) => ({
			...n,
			read_at: n.read_at ?? new Date().toISOString()
		}));
		unreadCount = 0;
		realtime.clearUnread();
	}

	function openNotification(n: Notification) {
		if (!n.read_at) {
			void markRead(n.id);
		}
		// Only navigate to in-app paths — goto() throws on absolute URLs.
		if (n.action_url && n.action_url.startsWith('/')) {
			goto(n.action_url);
		}
	}

	async function deleteNotification(n: Notification) {
		const index = notifications.findIndex((x) => x.id === n.id);
		if (index === -1) return;
		const removed = notifications[index];
		const next = [...notifications];
		next.splice(index, 1);
		notifications = next;
		const res = await api.delete(`/me/notifications/${n.id}`);
		if (res.error) {
			// Roll back the optimistic removal on API failure.
			notifications = [...notifications.slice(0, index), removed, ...notifications.slice(index)];
			return;
		}
		if (!removed.read_at) {
			unreadCount = Math.max(0, unreadCount - 1);
			realtime.decrementUnread();
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

<div class="mx-auto w-full max-w-full">
	<div class="sticky top-0 z-10 border-b border-[var(--color-border)] bg-[var(--color-surface)]" style="margin-inline: max(1rem, calc((100% - 48rem)/2))">
		<div class="flex items-center justify-between px-4 py-3">
			<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
				<Bell size={20} />
				{t('notifications.heading', 'Notifications')}
				{#if unreadCount > 0}
					<span
						class="rounded-full bg-[var(--color-primary)] px-2 py-0.5 text-xs font-medium text-[var(--color-primary-contrast)]"
					>
						{unreadCount}
					</span>
				{/if}
			</h1>
			<div class="flex items-center gap-2">
				{#if notifications.length > 0 && unreadCount > 0}
					<button
						type="button"
						onclick={markAllRead}
						disabled={markingAll}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					>
						<CheckCheck size={14} />
						{t('notifications.mark_all_read', 'Mark all read')}
					</button>
				{/if}
				<a
					href="/settings/notifications"
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] px-3 py-1.5 text-xs font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					aria-label={t('notifications.preferences', 'Notification settings')}
					title={t('notifications.preferences', 'Notification settings')}
				>
					<Settings size={16} />
					<span class="hidden sm:inline">{t('notifications.preferences', 'Notification settings')}</span>
				</a>
			</div>
		</div>

		<!-- Type filter chips (Section 35 — Filter by type) -->
		<div
			class="flex items-center gap-1 overflow-x-auto border-t border-[var(--color-border)] px-2 py-1.5"
			role="tablist"
			aria-label={t('common.filter', 'Filter')}
		>
			{#each NOTIFICATION_FILTER_TABS as tab (tab.key)}
				<button
					type="button"
					onclick={() => setFilter(tab.key)}
					role="tab"
					aria-selected={activeFilter === tab.key}
					class="inline-flex shrink-0 items-center gap-1 rounded-full px-2.5 py-1.5 text-xs font-medium transition-colors {activeFilter ===
					tab.key
						? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
						: 'text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]'}"
				>
					<tab.icon size={13} />
					{t(tab.labelKey, tab.fallback)}
				</button>
			{/each}
		</div>
	</div>

	<div class="mx-auto w-full max-w-3xl p-4">
		{#if notifications.length === 0}
			<EmptyState
				icon={BellOff}
				title={t('notifications.empty', 'No notifications yet')}
				body={t('notifications.empty_body', 'When people interact with you, you will see it here.')}
			/>
		{:else if filteredNotifications.length === 0}
			<EmptyState
				icon={BellOff}
				title={t('notifications.empty_filtered', 'No notifications in this category yet')}
				body={t(
					'notifications.empty_filtered_body',
					'Try selecting another filter or load more notifications.'
				)}
				action={hasMore && !loadingMore
					? {
							label: t('notifications.load_more', 'Load more'),
							onclick: loadMore
					  }
					: undefined}
			/>
			{#if loadingMore}
				<div class="flex items-center justify-center px-3 py-4 text-[var(--color-muted)]">
					<Loader2 size={18} class="animate-spin" />
				</div>
			{/if}
		{:else}
			<ul
				class="overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]"
				role="list"
			>
				{#each filteredNotifications as n (n.id)}
					<NotificationItem {n} onOpen={openNotification} onDelete={deleteNotification} />
				{/each}
			</ul>
			{#if hasMore && !loadingMore}
				<div use:infiniteScroll class="h-1 w-full" aria-hidden="true"></div>
			{/if}
			{#if loadingMore}
				<div class="flex items-center justify-center px-3 py-4 text-[var(--color-muted)]">
					<Loader2 size={18} class="animate-spin" />
				</div>
			{/if}
		{/if}
	</div>
</div>

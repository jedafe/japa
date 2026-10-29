<script lang="ts">
	/**
	 * NotificationCentre — Phase 13.01 dropdown (desktop) / slide-over (mobile).
	 *
	 * Triggered from the Notifications item in <LeftNav>. On open it fetches
	 * GET /me/notifications?page_size=20, renders the list with AutoAnimate,
	 * and offers "mark all read" (POST /me/notifications/read-all). Live
	 * notifications are prepended via the realtime store's onNotification hook.
	 *
	 * Ref: japaconnect-engineering/sections/35-notification-system.md
	 *      japaconnect-engineering/sections/23-main-site-frontend.md (Notification Centre)
	 */
	import { onMount, onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { Menu } from '$lib/components/ui';
	import { Bell, CheckCheck, BellOff, Loader2, X } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { realtime } from '$lib/realtime/realtime.svelte';
	import { EmptyState } from '$lib/components/ui';
	import {
		NOTIFICATION_FILTER_TABS,
		notificationFilterFor,
		notificationIconFor,
		type NotificationFilterTab
	} from '$lib/components/notifications/notificationType';
	import type { Notification } from '$lib/types';

	interface Props {
		/** Anchor side for the dropdown panel (desktop right, mobile can flip). */
		side?: 'right' | 'left' | 'top' | 'bottom';
		align?: 'start' | 'center' | 'end';
		/** Whether the left nav is collapsed (affects trigger rendering). */
		collapsed?: boolean;
		/** Optional CSS class for the trigger. */
		class?: string;
		/** Trigger content snippet. */
		children?: import('svelte').Snippet;
	}

	let { side = 'right', align = 'start', collapsed = false, class: klass = '', children }: Props = $props();

	let notifications = $state<Notification[]>([]);
	let loaded = $state(false);
	let loading = $state(false);
	let markingAll = $state(false);
	let open = $state(false);
	// BUG-015: infinite scroll state (§35 L96 — "Infinite scroll within the dropdown").
	let page = $state(0);
	let hasMore = $state(true);
	let loadingMore = $state(false);
	// Section 35 filter-by-type tabs.
	let activeFilter = $state<NotificationFilterTab>('all');

	const filteredNotifications = $derived(
		activeFilter === 'all'
			? notifications
			: notifications.filter((n) => notificationFilterFor(n.notification_type) === activeFilter)
	);

	// Live-prepend handler — registered on mount, fires whenever a `notification`
	// WS event arrives (regardless of whether the dropdown is open).
	function handleLiveNotification(n: Partial<Notification> & { id?: string }) {
		if (!n.id) return;
		// The same event can be delivered twice (reconnect replay) or arrive
		// after the row was already fetched — never duplicate a key.
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
	}

	onMount(() => {
		realtime.onNotification(handleLiveNotification);
	});
	onDestroy(() => {
		realtime.offNotification(handleLiveNotification);
	});

	/** Fetch the first page when the dropdown opens (lazy — no SSR fetch). */
	async function loadOnOpen() {
		// `loaded` — not `notifications.length` — gates the fetch: a live
		// prepend can populate the list before the dropdown is ever opened,
		// and skipping the fetch then would show only realtime rows forever.
		if (loaded || loading) return;
		loading = true;
		page = 0;
		hasMore = true;
		const res = await api.getEnvelope<Notification[]>(`/me/notifications?page_size=20`);
		if (res.data) {
			// Merge with any realtime rows that arrived before the first fetch.
			const fetched = new Set(res.data.map((x) => x.id));
			notifications = [
				...notifications.filter((x) => !fetched.has(x.id)),
				...res.data
			];
			// §35: prefer the server's has_more sidecar; fall back to the
			// short-page heuristic when the field is absent.
			const serverHasMore = res.envelope?.['has_more'];
			hasMore = typeof serverHasMore === 'boolean' ? serverHasMore : res.data.length >= 20;
			loaded = true;
		}
		loading = false;
	}

	/**
	 * BUG-015: load the next page of notifications for infinite scroll.
	 * §35 L96: "Infinite scroll within the dropdown".
	 */
	async function loadMore() {
		if (loadingMore || !hasMore || loading) return;
		loadingMore = true;
		page += 1;
		const offset = page * 20;
		const res = await api.getEnvelope<Notification[]>(
			`/me/notifications?page_size=20&page_offset=${offset}`
		);
		if (res.data) {
			// Live prepends shift the server-side offset window, so a page can
			// return rows we already have — drop duplicates to keep the keyed
			// each-block valid.
			const seen = new Set(notifications.map((x) => x.id));
			const fresh = res.data.filter((x) => !seen.has(x.id));
			notifications = [...notifications, ...fresh];
			const serverHasMore = res.envelope?.['has_more'];
			hasMore = typeof serverHasMore === 'boolean' ? serverHasMore : res.data.length >= 20;
		} else {
			hasMore = false;
		}
		loadingMore = false;
	}

	/** Mark a single notification read and navigate to its action URL. */
	async function openNotification(n: Notification) {
		if (!n.read_at) {
			// Optimistic — don't block navigation on the API call.
			notifications = notifications.map((x) =>
				x.id === n.id ? { ...x, read_at: new Date().toISOString() } : x
			);
			realtime.decrementUnread(1);
			const res = await api.post(`/me/notifications/${n.id}/read`);
			if (res.error) {
				// Roll back the optimistic read so the unread state stays true.
				notifications = notifications.map((x) =>
					x.id === n.id ? { ...x, read_at: null } : x
				);
				realtime.incrementUnread(1);
			}
		}
		// Only navigate to in-app paths — an absolute/external action_url would
		// throw inside goto(). Non-path values are ignored safely.
		if (n.action_url && n.action_url.startsWith('/')) goto(n.action_url);
		open = false;
	}

	/** BUG-025: per-item delete inside the dropdown (parity with /notifications). */
	async function deleteNotification(n: Notification) {
		const index = notifications.findIndex((x) => x.id === n.id);
		if (index === -1) return;
		const removed = notifications[index];
		const next = [...notifications];
		next.splice(index, 1);
		notifications = next;
		const res = await api.delete(`/me/notifications/${n.id}`);
		if (res.error) {
			notifications = [...notifications.slice(0, index), removed, ...notifications.slice(index)];
			return;
		}
		if (!removed.read_at) realtime.decrementUnread();
	}

	/** Mark all visible notifications as read. */
	async function markAllRead() {
		if (markingAll) return;
		markingAll = true;
		const res = await api.post(`/me/notifications/read-all`);
		if (!res.error) {
			notifications = notifications.map((x) => ({
				...x,
				read_at: x.read_at ?? new Date().toISOString()
			}));
			realtime.clearUnread();
		}
		markingAll = false;
	}

	const unread = $derived(notifications.filter((n) => !n.read_at).length);
	const badge = $derived(realtime.unreadNotifications);

	/**
	 * BUG-015: Svelte action that triggers loadMore when the sentinel
	 * element scrolls into view. Uses IntersectionObserver for efficient
	 * scroll detection without polling. §35 L96.
	 */
	function infiniteScroll(node: HTMLElement) {
		const observer = new IntersectionObserver(
			(entries) => {
				if (entries[0]?.isIntersecting) {
					loadMore();
				}
			},
			{ root: node.closest('.overflow-y-auto'), threshold: 0.1 }
		);
		observer.observe(node);
		return {
			destroy() {
				observer.disconnect();
			}
		};
	}
</script>

<Menu bind:open onOpenChange={(isOpen) => { if (isOpen) loadOnOpen(); }} {side} {align} padded={false} contentClass="max-h-[70vh] w-80 overflow-y-auto">
	{#snippet trigger(props)}
		<button
			type="button"
			{...props}
			class={children
				? `w-full text-start ${klass}`
				: 'grid h-9 w-9 shrink-0 place-items-center rounded-full text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]'}
			aria-label={t('nav.notifications')}
		>
			{#if children}
				{@render children()}
			{:else}
				<span class="relative">
					<Bell size={18} />
					{#if badge > 0}
						<span
							class="absolute -end-1.5 -top-1.5 grid h-4 min-w-4 place-items-center rounded-full bg-[var(--color-primary)] px-0.5 text-[9px] font-semibold text-[var(--color-primary-contrast)]"
						>{badge > 99 ? '99+' : badge}</span>
					{/if}
				</span>
			{/if}
		</button>
	{/snippet}
			<!-- Header -->
			<div class="flex items-center justify-between border-b border-[var(--color-border)] px-3 py-2">
				<span class="text-sm font-medium text-[var(--color-text)]">
					{t('notifications.heading', 'Notifications')}
					{#if unread > 0}
						<span class="ms-1 rounded-full bg-[var(--color-primary)] px-1.5 py-0.5 text-[10px] font-semibold text-[var(--color-primary-contrast)]">
							{unread > 99 ? '99+' : unread}
						</span>
					{/if}
				</span>
				<button
					type="button"
					onclick={markAllRead}
					disabled={markingAll || unread === 0}
					class="inline-flex items-center gap-1 rounded-[var(--radius)] px-2 py-1 text-xs text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					aria-label={t('notifications.mark_all_read', 'Mark all as read')}
				>
					{#if markingAll}
						<Loader2 size={12} class="animate-spin" />
					{:else}
						<CheckCheck size={12} />
					{/if}
					{t('notifications.mark_all_read', 'Mark all read')}
				</button>
			</div>

			<!-- Filter tabs (Section 35 — Filter by type) -->
			<div class="flex items-center gap-1 overflow-x-auto border-b border-[var(--color-border)] px-2 py-1.5">
				{#each NOTIFICATION_FILTER_TABS as tab (tab.key)}
					<button
						type="button"
						onclick={() => (activeFilter = tab.key)}
						class="inline-flex shrink-0 items-center gap-1 rounded-full px-2 py-1 text-[11px] font-medium transition-colors {activeFilter === tab.key ? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]' : 'text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]'}"
						aria-label={t(tab.labelKey, tab.fallback)}
					>
						<tab.icon size={11} />
						{t(tab.labelKey, tab.fallback)}
					</button>
				{/each}
			</div>

			<!-- List -->
			<div class="divide-y divide-[var(--color-border)]">
				{#if loading}
					<div class="flex items-center justify-center px-3 py-8 text-[var(--color-muted)]">
						<Loader2 size={16} class="animate-spin" />
					</div>
				{:else if filteredNotifications.length === 0}
					<div class="px-3 py-8">
						<EmptyState icon={BellOff} title={t('notifications.empty', 'No notifications yet')} />
					</div>
				{:else}
					{#each filteredNotifications as n (n.id)}
						{@const Icon = notificationIconFor(n.notification_type)}
						<div class="flex items-start">
						<button
							type="button"
							onclick={() => openNotification(n)}
							class="flex w-full items-start gap-2 px-3 py-2.5 text-start transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							<span class="mt-0.5 grid h-7 w-7 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]">
								<Icon size={13} />
							</span>
							<span class="min-w-0 flex-1">
								<span class="flex items-center gap-1.5">
									<span class="truncate text-xs font-medium text-[var(--color-text)]">{n.title || n.type_label}</span>
									{#if !n.read_at}
										<span class="h-1.5 w-1.5 shrink-0 rounded-full bg-[var(--color-primary)]" aria-label={t('notifications.unread', 'Unread')}></span>
									{/if}
								</span>
								{#if n.body}
									<span class="mt-0.5 line-clamp-2 block text-xs text-[var(--color-text-subtle)]">{n.body}</span>
								{/if}
							</span>
							</button>
							<button
								type="button"
								onclick={() => deleteNotification(n)}
								class="me-2 mt-2.5 grid h-5 w-5 shrink-0 place-items-center rounded-full text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-destructive)]"
								aria-label={t('common.remove', 'Remove')}
							>
								<X size={12} />
							</button>
						</div>
					{/each}
					<!-- BUG-015: infinite scroll sentinel (§35 L96). -->
					{#if hasMore && !loadingMore}
						<div
							use:infiniteScroll
							class="h-1 w-full"
							aria-hidden="true"
						></div>
					{/if}
					{#if loadingMore}
						<div class="flex items-center justify-center px-3 py-3 text-[var(--color-muted)]">
							<Loader2 size={14} class="animate-spin" />
						</div>
					{/if}
				{/if}
			</div>

			<!-- Footer -->
			<div class="border-t border-[var(--color-border)] px-3 py-2">
				<a
					href="/notifications"
					class="block text-center text-xs font-medium text-[var(--color-primary)] hover:underline"
					onclick={() => (open = false)}
				>
					{t('notifications.view_all', 'View all notifications')}
				</a>
			</div>
</Menu>

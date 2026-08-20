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
	import { DropdownMenu } from 'bits-ui';
	import { Bell, CheckCheck, BellOff, Loader2 } from 'lucide-svelte';
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
		if (notifications.length > 0 || loading) return;
		loading = true;
		page = 0;
		hasMore = true;
		const res = await api.get<Notification[]>(`/me/notifications?page_size=20`);
		if (res.data) {
			notifications = res.data;
			// BUG-015: if the API returned fewer than the page size, there's no more.
			if (res.data.length < 20) hasMore = false;
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
		const res = await api.get<Notification[]>(`/me/notifications?page_size=20&page_offset=${offset}`);
		if (res.data) {
			notifications = [...notifications, ...res.data];
			if (res.data.length < 20) hasMore = false;
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
			void api.post(`/me/notifications/${n.id}/read`);
		}
		if (n.action_url) goto(n.action_url);
		open = false;
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

<DropdownMenu.Root bind:open onOpenChange={(isOpen) => { if (isOpen) loadOnOpen(); }}>
	<DropdownMenu.Trigger
		class="w-full text-left {klass}"
		aria-label={t('nav.notifications')}
	>
		{@render children?.()}
	</DropdownMenu.Trigger>
	<DropdownMenu.Portal>
		<DropdownMenu.Content
			{side}
			{align}
			sideOffset={8}
			class="z-50 max-h-[70vh] w-80 overflow-y-auto rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-0 shadow-lg"
		>
			<!-- Header -->
			<div class="flex items-center justify-between border-b border-[var(--color-border)] px-3 py-2">
				<span class="text-sm font-medium text-[var(--color-text)]">
					{t('notifications.heading', 'Notifications')}
					{#if unread > 0}
						<span class="ml-1 rounded-full bg-[var(--color-primary)] px-1.5 py-0.5 text-[10px] font-semibold text-[var(--color-primary-contrast)]">
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
						<button
							type="button"
							onclick={() => openNotification(n)}
							class="flex w-full items-start gap-2 px-3 py-2.5 text-left transition-colors hover:bg-[var(--color-surface-subtle)]"
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
		</DropdownMenu.Content>
	</DropdownMenu.Portal>
</DropdownMenu.Root>

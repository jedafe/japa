/**
 * Real-time store — bridges the WsClient to reactive Svelte 5 Runes state.
 *
 * Learn only needs the unread-notifications badge (no DMs, feed updates, or
 * space subscriptions — those are main-app concerns). This is a deliberately
 * smaller surface than apps/main's realtime store.
 *
 * Wired in the (app) layout: `realtime.connect()` on auth, `realtime.disconnect()`
 * on unmount. All state is client-only; the store is a no-op during SSR.
 */

import { wsClient, type WSEventType } from './wsClient';
import type { Notification } from '$lib/types';

type NotificationHandler = (n: Partial<Notification> & { id?: string }) => void;

class RealtimeStore {
	/** Unread notification count for the nav badge. -1 = unknown (not yet loaded). */
	unreadNotifications = $state<number>(0);

	private notificationHandlers = new Set<NotificationHandler>();
	private wired = false;

	private readonly onNotificationBound = (p: unknown) => this.handleNotification(p);

	/** Connect the WebSocket and register event handlers. Idempotent. */
	connect(): void {
		if (this.wired) return;
		this.wired = true;
		wsClient.on('notification' as WSEventType, this.onNotificationBound);
		wsClient.connect();
	}

	disconnect(): void {
		if (!this.wired) return;
		this.wired = false;
		wsClient.off('notification' as WSEventType, this.onNotificationBound);
		wsClient.disconnect();
	}

	/** Set the unread count after a REST fetch (GET /me/notifications/unread-count). */
	setUnread(count: number): void {
		this.unreadNotifications = Math.max(0, count);
	}

	onNotification(cb: NotificationHandler): void {
		this.notificationHandlers.add(cb);
	}

	offNotification(cb: NotificationHandler): void {
		this.notificationHandlers.delete(cb);
	}

	private handleNotification(payload: unknown): void {
		const n = payload as Partial<Notification> & { id?: string };
		this.unreadNotifications += 1;
		for (const cb of this.notificationHandlers) cb(n);
	}
}

export const realtime = new RealtimeStore();

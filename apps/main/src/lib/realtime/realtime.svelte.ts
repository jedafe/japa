/**
 * Real-time store — bridges the WsClient to reactive Svelte 5 Runes state.
 *
 * Wired in the (app) layout: `realtime.connect()` on auth, `realtime.disconnect()`
 * on unmount. Surfaces:
 *   - `unreadNotifications` — drives the LeftNav badge + notifications page live prepend
 *   - `onlineUserIds` — presence set for typing/online indicators
 *   - `feedUpdateCount` — drives FeedUpdatePill on the home timeline
 *   - `onDMMessage(cb)` / `offDMMessage(cb)` — live DM delivery hook for /messages
 *   - `onNotification(cb)` / `offNotification(cb)` — live notification hook for /notifications
 *   - `subscribeSpace(id)` / `unsubscribeSpace(id)` — space activity subscriptions
 *
 * All state is client-only; the store is a no-op during SSR.
 */

import { wsClient, type WSEventType } from './wsClient';
import type { Notification } from '$lib/types';

/** Payload shape broadcast by the backend for `dm_message` events. */
export interface DMMessageEvent {
	conversation_id: string;
	sender_id: string;
	body: string;
}

/** Payload shape broadcast by the backend for `presence` events. */
export interface PresenceEvent {
	user_id: string;
	status: 'online' | 'offline';
}

type DMHandler = (ev: DMMessageEvent) => void;
type NotificationHandler = (n: Partial<Notification> & { id?: string }) => void;

/** Payload shape broadcast by the backend for `poll_updated` events. */
export interface PollUpdatedEvent {
	poll_id: string;
	results: Array<{ option_id: string; label: string; position: number; vote_count: number }>;
}

class RealtimeStore {
	/** Unread notification count for the nav badge. -1 = unknown (not yet loaded). */
	unreadNotifications = $state<number>(0);

	/** Number of new feed items since the user last dismissed the pill. */
	feedUpdateCount = $state<number>(0);

	/** Set of user IDs currently online (presence). */
	onlineUserIds = $state<Set<string>>(new Set());

	private dmHandlers = new Set<DMHandler>();
	private notificationHandlers = new Set<NotificationHandler>();
	private pollHandlers = new Set<(ev: PollUpdatedEvent) => void>();
	private wired = false;

	// Bound handler references so on()/off() match the same function identity.
	private readonly onNotificationBound = (p: unknown) => this.handleNotification(p);
	private readonly onPresenceBound = (p: unknown) => this.handlePresence(p);
	private readonly onFeedUpdateBound = () => this.handleFeedUpdate();
	private readonly onDMMessageBound = (p: unknown) => this.handleDMMessage(p);
	private readonly onPollUpdatedBound = (p: unknown) => this.handlePollUpdated(p);
	private readonly onModerationAlertBound = (p: unknown) => this.handleModerationAlert(p);

	/** Connect the WebSocket and register event handlers. Idempotent. */
	connect(): void {
		if (!this.wired) {
			this.wired = true;

			wsClient.on('notification', this.onNotificationBound);
			wsClient.on('presence', this.onPresenceBound);
			wsClient.on('feed_update', this.onFeedUpdateBound);
			wsClient.on('dm_message', this.onDMMessageBound);
			wsClient.on('poll_updated', this.onPollUpdatedBound);
			wsClient.on('moderation_alert', this.onModerationAlertBound);
		}

		// Always re-arm the client, not just on first wiring: wsClient.connect()
		// is idempotent while a socket is open or a mint is in flight, so this
		// is cheap — and it lets a session that recovered from an auth lapse
		// (e.g. token refreshed by another request after a failed mint)
		// re-establish the socket on the next connect() call.
		void wsClient.connect();
	}

	/** Disconnect the WebSocket and clear handlers. */
	disconnect(): void {
		if (!this.wired) return;
		this.wired = false;
		wsClient.off('notification', this.onNotificationBound);
		wsClient.off('presence', this.onPresenceBound);
		wsClient.off('feed_update', this.onFeedUpdateBound);
		wsClient.off('dm_message', this.onDMMessageBound);
		wsClient.off('poll_updated', this.onPollUpdatedBound);
		wsClient.off('moderation_alert', this.onModerationAlertBound);
		wsClient.disconnect();
	}

	/** Seed the unread count from the SSR-loaded notifications page. */
	setUnreadCount(n: number): void {
		this.unreadNotifications = Math.max(0, n);
	}

	/** Decrement the unread count (e.g. after the user opens a notification). */
	decrementUnread(by = 1): void {
		this.unreadNotifications = Math.max(0, this.unreadNotifications - by);
	}

	/** Reset unread to zero (e.g. after "mark all read"). */
	clearUnread(): void {
		this.unreadNotifications = 0;
	}

	/** Dismiss the feed update pill and reset the counter. */
	dismissFeedUpdatePill(): void {
		this.feedUpdateCount = 0;
	}

	/** Subscribe to a space's live updates (forwarded to the backend). */
	subscribeSpace(spaceId: string): void {
		wsClient.subscribeSpace(spaceId);
	}

	/** Unsubscribe from a space's live updates. */
	unsubscribeSpace(spaceId: string): void {
		wsClient.unsubscribeSpace(spaceId);
	}

	/** Register a handler for live DM messages (used by /messages). */
	onDMMessage(handler: DMHandler): void {
		this.dmHandlers.add(handler);
	}

	/** Remove a previously-registered DM handler. */
	offDMMessage(handler: DMHandler): void {
		this.dmHandlers.delete(handler);
	}

	/** Register a handler for live poll update events (used by PollBlock). */
	onPollUpdated(handler: (ev: PollUpdatedEvent) => void): void {
		this.pollHandlers.add(handler);
	}

	/** Remove a previously-registered poll update handler. */
	offPollUpdated(handler: (ev: PollUpdatedEvent) => void): void {
		this.pollHandlers.delete(handler);
	}

	/** Register a handler for live notifications (used by /notifications). */
	onNotification(handler: NotificationHandler): void {
		this.notificationHandlers.add(handler);
	}

	/** Remove a previously-registered notification handler. */
	offNotification(handler: NotificationHandler): void {
		this.notificationHandlers.delete(handler);
	}

	// --- internal event handlers ---

	private handleNotification(payload: unknown): void {
		const n = (payload ?? {}) as Partial<Notification> & { id?: string };
		this.unreadNotifications += 1;
		for (const h of this.notificationHandlers) {
			try {
				h(n);
			} catch {
				/* swallow */
			}
		}
	}

	private handlePresence(payload: unknown): void {
		const ev = (payload ?? {}) as PresenceEvent;
		if (!ev.user_id) return;
		const next = new Set(this.onlineUserIds);
		if (ev.status === 'offline') next.delete(ev.user_id);
		else next.add(ev.user_id);
		this.onlineUserIds = next;
	}

	private handleFeedUpdate(): void {
		this.feedUpdateCount += 1;
	}

	private handlePollUpdated(payload: unknown): void {
		const ev = (payload ?? {}) as PollUpdatedEvent;
		if (!ev.poll_id) return;
		for (const h of this.pollHandlers) {
			try {
				h(ev);
			} catch {
				/* swallow */
			}
		}
	}

	private handleDMMessage(payload: unknown): void {
		const ev = (payload ?? {}) as DMMessageEvent;
		if (!ev.conversation_id) return;
		for (const h of this.dmHandlers) {
			try {
				h(ev);
			} catch {
				/* swallow */
			}
		}
	}

	private handleModerationAlert(_payload: unknown): void {
		// Moderation alerts are surfaced via the notifications stream; no
		// dedicated UI surface yet. Hook left for future use.
	}
}

export const realtime = new RealtimeStore();

// Re-export the event type for consumers that need to register custom handlers.
export type { WSEventType };

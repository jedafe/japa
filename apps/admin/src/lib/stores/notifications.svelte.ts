import { browser } from '$app/environment';
import { apiFetch } from '$lib/api/client';

export interface AdminNotification {
	id: string;
	title: string;
	body: string;
	type: 'info' | 'warning' | 'error' | 'success';
	read: boolean;
	created_at: string;
	link?: string;
}

/** Raw row shape returned by GET /me/notifications (§35). */
interface ApiNotificationRow {
	id: string;
	notification_type: string;
	title: string;
	body: string | null;
	action_url: string | null;
	read_at: string | null;
	created_at: string;
	type_label?: string;
}

/** Map a backend notification row onto the UI-facing AdminNotification. */
export function mapApiNotification(row: ApiNotificationRow): AdminNotification {
	return {
		id: row.id,
		title: row.title ?? '',
		body: row.body ?? '',
		type: 'info',
		read: row.read_at != null,
		created_at: row.created_at,
		link: row.action_url ?? undefined
	};
}

let _notifications = $state<AdminNotification[]>([]);
export function getUnreadCount(): number {
	return _notifications.filter((n) => !n.read).length;
}

let initialized = false;
let pollInterval: ReturnType<typeof setInterval> | undefined;

export function initNotifications(): (() => void) | void {
	if (initialized || !browser) return;
	initialized = true;

	fetchNotifications();
	pollInterval = setInterval(fetchNotifications, 60_000);

	return () => {
		if (pollInterval) {
			clearInterval(pollInterval);
			pollInterval = undefined;
		}
		// Reset so a subsequent login in the same page session re-initializes
		// the poller — otherwise notifications silently stop after logout.
		initialized = false;
	};
}

async function fetchNotifications() {
	try {
		// Backend paginates with page_size/page_offset — `limit` is ignored.
		const result = await apiFetch<ApiNotificationRow[]>('/me/notifications?page_size=20');
		if (!result.error && result.data) {
			// Keep local (WS-generated) notifications that the API doesn't
			// know about — they carry no server id.
			const apiIds = new Set(result.data.map((r) => r.id));
			const local = _notifications.filter((n) => !apiIds.has(n.id) && n.id.startsWith('local-'));
			_notifications = [...local, ...result.data.map(mapApiNotification)].slice(0, 50);
		}
	} catch {
		// degrade gracefully
	}
}

export function addNotification(notification: Omit<AdminNotification, 'id' | 'read' | 'created_at'>) {
	// Local-only notifications (WS events, 403 warnings) get a local- id so
	// fetchNotifications can distinguish them from server rows — markRead
	// against /me/notifications/{id} would 500 on a non-UUID otherwise.
	const id = `local-${crypto.randomUUID()}`;
	const newNotif: AdminNotification = {
		...notification,
		id,
		read: false,
		created_at: new Date().toISOString()
	};
	_notifications = [newNotif, ..._notifications].slice(0, 50);
}

export async function markRead(id: string) {
	_notifications = _notifications.map((n) => (n.id === id ? { ...n, read: true } : n));
	// Local notifications (WS events, permission warnings) have no server row.
	if (id.startsWith('local-')) return;
	try {
		const res = await apiFetch(`/me/notifications/${id}/read`, { method: 'POST' });
		if (res.error) {
			// Roll back the optimistic read on failure.
			_notifications = _notifications.map((n) => (n.id === id ? { ...n, read: false } : n));
		}
	} catch {
		// best effort
	}
}

export async function markAllRead() {
	_notifications = _notifications.map((n) => ({ ...n, read: true }));
	try {
		await apiFetch('/me/notifications/read-all', { method: 'POST' });
	} catch {
		// best effort
	}
}

export function getNotifications(): AdminNotification[] {
	return _notifications;
}

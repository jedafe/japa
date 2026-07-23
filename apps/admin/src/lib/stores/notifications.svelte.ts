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
	};
}

async function fetchNotifications() {
	try {
		const result = await apiFetch<AdminNotification[]>('/me/notifications?limit=20');
		if (!result.error && result.data) {
			_notifications = result.data;
		}
	} catch {
		// degrade gracefully
	}
}

export function addNotification(notification: Omit<AdminNotification, 'id' | 'read' | 'created_at'>) {
	const id = crypto.randomUUID();
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
	try {
		await apiFetch(`/me/notifications/${id}/read`, { method: 'POST' });
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

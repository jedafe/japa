import { browser } from '$app/environment';
import { PUBLIC_JC_API_URL } from '$env/static/public';

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

export function initNotifications() {
	if (initialized || !browser) return;
	initialized = true;

	fetchNotifications();
	setInterval(fetchNotifications, 60_000);
}

async function fetchNotifications() {
	try {
		const res = await fetch(`${PUBLIC_JC_API_URL}/api/v1/me/notifications?limit=20`, {
			credentials: 'include'
		});
		if (res.ok) {
			const { data } = await res.json();
			_notifications = data ?? [];
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
		await fetch(`${PUBLIC_JC_API_URL}/api/v1/me/notifications/${id}/read`, {
			method: 'POST',
			credentials: 'include'
		});
	} catch {
		// best effort
	}
}

export async function markAllRead() {
	_notifications = _notifications.map((n) => ({ ...n, read: true }));
	try {
		await fetch(`${PUBLIC_JC_API_URL}/api/v1/me/notifications/read-all`, {
			method: 'POST',
			credentials: 'include'
		});
	} catch {
		// best effort
	}
}

export function getNotifications(): AdminNotification[] {
	return _notifications;
}

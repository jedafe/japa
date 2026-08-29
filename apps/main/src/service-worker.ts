/// <reference types="@sveltejs/kit" />
/// <reference lib="webworker" />

// Service worker for browser push notifications (Section 35 / Section 50).
// Ref: japaconnect-engineering/sections/35-notification-system.md

const sw = self as unknown as ServiceWorkerGlobalScope;

// Take control of clients immediately so push events can open the app.
sw.addEventListener('install', () => {
	sw.skipWaiting();
});

sw.addEventListener('activate', (event) => {
	event.waitUntil(sw.clients.claim());
});

// Push events carry a JSON payload with { title, body, action_url }.
sw.addEventListener('push', (event) => {
	let payload: { title?: string; body?: string; action_url?: string } = {};
	try {
		payload = event.data?.json() ?? {};
	} catch {
		// non-JSON payload — fall back to text.
		payload = { body: event.data?.text() ?? '' };
	}

	const title = payload.title ?? 'JapaConnect';
	const body = payload.body ?? '';
	const actionUrl = payload.action_url ? new URL(payload.action_url, sw.location.origin).toString() : sw.location.origin;

	const options: NotificationOptions = {
		body,
		data: { url: actionUrl },
		icon: '/favicon.png',
		badge: '/favicon.png'
	};

	event.waitUntil(sw.registration.showNotification(title, options));
});

// Clicking a push notification focuses/closes the relevant client.
sw.addEventListener('notificationclick', (event) => {
	event.notification.close();
	const url = (event.notification.data as { url?: string } | undefined)?.url ?? sw.location.origin;
	event.waitUntil(
		sw.clients
			.matchAll({ type: 'window', includeUncontrolled: true })
			.then((clientList) => {
				for (const client of clientList) {
					if (client.url === url && 'focus' in client) {
						return client.focus();
					}
				}
				if (sw.clients.openWindow) {
					return sw.clients.openWindow(url);
				}
			})
	);
});

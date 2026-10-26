/// <reference types="@sveltejs/kit" />
/// <reference lib="webworker" />

// Service worker: browser push notifications (Section 35 / Section 50) plus
// the §124 PWA caching layer (precache, runtime caching, offline fallback).
// Ref: japaconnect-engineering/sections/35-notification-system.md
// Ref: .jeos/sections/124-progressive-web-app.md

import { PUBLIC_JC_API_URL } from '$env/static/public';
import { version } from '$service-worker';
import { buildApiUrl } from '@japaconnect/api-url';
import { initPwaServiceWorker } from '@japaconnect/ui/pwa';
import { DEFAULT_SITE_NAME } from '$lib/brand';

const sw = self as unknown as ServiceWorkerGlobalScope;

// §124 — offline caching + fallback. Listeners are additive: the push
// handlers below are unaffected. `version` changes every build, so cache
// namespaces roll and the worker re-installs on each deploy. The API host
// is cross-origin, so API responses are never cached.
initPwaServiceWorker(sw, {
	version,
	settingsUrl: buildApiUrl('/site/settings/public', PUBLIC_JC_API_URL),
	precacheExtras: ['/favicon.png', '/icon-192.png']
});

// Notification title fallback comes from general.site_name — fetched lazily
// on the first push (activate is too early: no network guarantee) and cached
// for the SW's lifetime. Never hardcode the brand here.
let cachedSiteName = DEFAULT_SITE_NAME;
let siteNameFetched = false;
async function refreshSiteName(): Promise<void> {
	if (siteNameFetched) return;
	siteNameFetched = true;
	try {
		const res = await fetch(buildApiUrl('/site/settings/public', PUBLIC_JC_API_URL));
		if (!res.ok) return;
		const body = await res.json();
		const name = (body?.data ?? body)?.['general.site_name'];
		if (typeof name === 'string' && name) cachedSiteName = name;
	} catch {
		// Offline or transient failure — keep the default.
	}
}

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
	const body = payload.body ?? '';
	// Same-origin gate: a compromised notifier (or an API echoing a crafted
	// content URL into the push payload) must not turn a trusted
	// notification into a one-click phish. Cross-origin action_urls fall
	// back to the app origin.
	let actionUrl = sw.location.origin;
	if (payload.action_url) {
		try {
			const parsed = new URL(payload.action_url, sw.location.origin);
			if (parsed.origin === sw.location.origin) {
				actionUrl = parsed.toString();
			}
		} catch {
			// malformed URL — keep the origin fallback
		}
	}

	const options: NotificationOptions = {
		body,
		data: { url: actionUrl },
		icon: '/favicon.png',
		badge: '/favicon.png'
	};

	event.waitUntil(
		(async () => {
			if (!payload.title) await refreshSiteName();
			await sw.registration.showNotification(payload.title ?? cachedSiteName, options);
		})()
	);
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

/// <reference types="@sveltejs/kit" />
/// <reference lib="webworker" />

// §124 — PWA service worker for the Academy surface: precache + runtime
// caching + offline fallback. No push handlers (browser push subscriptions
// are managed on the main app's Devices settings page).
// Ref: .jeos/sections/124-progressive-web-app.md

import { PUBLIC_JC_API_URL } from '$env/static/public';
import { version } from '$service-worker';
import { buildApiUrl } from '@japaconnect/api-url';
import { initPwaServiceWorker } from '@japaconnect/ui/pwa';

const sw = self as unknown as ServiceWorkerGlobalScope;

initPwaServiceWorker(sw, {
	version,
	settingsUrl: buildApiUrl('/site/settings/public', PUBLIC_JC_API_URL),
	precacheExtras: ['/favicon.png', '/icon-192.png']
});

sw.addEventListener('install', () => {
	sw.skipWaiting();
});

sw.addEventListener('activate', (event) => {
	event.waitUntil(sw.clients.claim());
});

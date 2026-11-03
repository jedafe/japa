/// <reference types="@sveltejs/kit" />
/// <reference lib="webworker" />

// §124 — PWA service worker for the Academy surface: precache + runtime
// caching + offline fallback. No push handlers (browser push subscriptions
// are managed on the main app's Devices settings page).
// Ref: .jeos/sections/124-progressive-web-app.md

import { PUBLIC_JC_API_URL } from '$env/static/public';
// §124 — build/start timestamp injected via vite `define`, used only for
// cache namespacing. `$service-worker` is deliberately NOT imported: its dev
// shim guards on `self instanceof ServiceWorkerGlobalScope`, which throws at
// evaluation time in browsers that don't expose that constructor in worker
// scope — the whole registration then fails ("script threw an exception
// during script evaluation").
declare const __SW_VERSION__: string | undefined;
const version = typeof __SW_VERSION__ === 'string' && __SW_VERSION__ ? __SW_VERSION__ : 'sw-dev';
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

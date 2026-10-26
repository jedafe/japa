/// <reference lib="webworker" />
/**
 * Shared Progressive Web App helpers for all JC SvelteKit surfaces (§124).
 *
 * Two exports, two consumers:
 *  - `buildWebManifest` — used by each app's `/manifest.json` +server.ts to
 *    compose the web app manifest from public `system_settings`.
 *  - `initPwaServiceWorker` — used by each app's `src/service-worker.ts` to
 *    wire precache/runtime caching and the offline fallback. Pure TS: no
 *    Svelte or SvelteKit imports so it bundles cleanly into a worker.
 *
 * Ref: .jeos/sections/124-progressive-web-app.md
 */

type Settings = Record<string, unknown>;

function str(settings: Settings, key: string): string {
	const v = settings[key];
	return typeof v === 'string' ? v.trim() : '';
}

function bool(settings: Settings, key: string, fallback: boolean): boolean {
	const v = settings[key];
	return typeof v === 'boolean' ? v : fallback;
}

// json-type settings (pwa.shortcuts, pwa.screenshots) arrive as an array when
// seeded, but as a JSON-encoded *string* when the admin saved them through a
// textarea — parse both forms or the value is silently dropped.
function jsonArray(settings: Settings, key: string): unknown[] {
	const v = settings[key];
	if (Array.isArray(v)) return v;
	if (typeof v === 'string') {
		try {
			const parsed = JSON.parse(v);
			return Array.isArray(parsed) ? parsed : [];
		} catch {
			return [];
		}
	}
	return [];
}

const HEX_COLOR = /^#[0-9a-fA-F]{3,8}$/;
function hexColor(settings: Settings, key: string): string {
	const v = str(settings, key);
	return HEX_COLOR.test(v) ? v : '';
}

const VALID_DISPLAY = new Set(['standalone', 'minimal-ui', 'browser', 'fullscreen']);

interface ManifestIcon {
	src: string;
	sizes: string;
	type: string;
	purpose?: string;
}

interface ManifestShortcut {
	name: string;
	url: string;
	icons?: ManifestIcon[];
}

export interface WebManifest {
	name: string;
	short_name: string;
	description: string;
	start_url: string;
	scope: string;
	display: string;
	orientation: string;
	background_color: string;
	theme_color: string;
	lang: string;
	dir: string;
	categories: string[];
	icons: ManifestIcon[];
	shortcuts?: ManifestShortcut[];
	screenshots?: unknown[];
	share_target?: {
		action: string;
		method: string;
		params: { title: string; text: string; url: string };
	};
}

export interface ManifestAppOptions {
	/** Appended to the resolved app name (e.g. ' Academy', ' Docs'). */
	nameSuffix?: string;
	/** Per-app short_name fallback when pwa.* settings are empty. */
	defaultShortName?: string;
	/** Per-app start_url fallback (e.g. '/' or '/docs'). */
	defaultStartUrl: string;
	/** Emit a share_target block when pwa.share_target_enabled is on (main only). */
	includeShareTarget?: boolean;
}

/**
 * Compose the web app manifest for one surface from the public site settings
 * map (GET /site/settings/public). Every pwa.* key falls back to a sensible
 * per-app default so the manifest is always valid even with no stored rows.
 */
export function buildWebManifest(settings: Settings, app: ManifestAppOptions): WebManifest {
	const siteName = str(settings, 'general.site_name') || 'JapaConnect';
	const name = `${str(settings, 'pwa.name') || siteName}${app.nameSuffix ?? ''}`;
	const shortName =
		str(settings, 'pwa.short_name') || str(settings, 'pwa.name') || app.defaultShortName || siteName;
	const description =
		str(settings, 'pwa.description') || str(settings, 'general.site_description');
	const themeColor =
		hexColor(settings, 'pwa.theme_color') ||
		hexColor(settings, 'appearance.primary_color') ||
		'#3b49df';

	const icon192 = str(settings, 'pwa.icon_192') || '/icon-192.png';
	const icon512 = str(settings, 'pwa.icon_512') || '/favicon.png';
	const maskable = str(settings, 'pwa.maskable_icon') || '/icon-maskable-512.png';

	const rawDisplay = str(settings, 'pwa.display');
	const display = VALID_DISPLAY.has(rawDisplay) ? rawDisplay : 'standalone';
	// start_url must be a same-origin path — an absolute URL would take the
	// installed app's launch out of its own scope.
	const rawStartUrl = str(settings, 'pwa.start_url');
	const startUrl = rawStartUrl.startsWith('/') ? rawStartUrl : app.defaultStartUrl;

	const categories = (str(settings, 'pwa.categories') || 'social')
		.split(',')
		.map((c) => c.trim())
		.filter(Boolean);

	const shortcuts: ManifestShortcut[] = jsonArray(settings, 'pwa.shortcuts')
		.filter((s): s is Record<string, unknown> => typeof s === 'object' && s !== null)
		.map((s) => ({
			// link-list editors emit {label, url}; the manifest field is `name`.
			name: (typeof s.name === 'string' && s.name) || (typeof s.label === 'string' && s.label) || '',
			url: typeof s.url === 'string' && s.url.startsWith('/') ? s.url : '/'
		}))
		.filter((s) => s.name !== '');

	const manifest: WebManifest = {
		name,
		short_name: shortName,
		description,
		start_url: startUrl,
		scope: '/',
		display,
		orientation: 'any',
		background_color: hexColor(settings, 'pwa.background_color') || '#ffffff',
		theme_color: themeColor,
		lang: str(settings, 'i18n.default_language') || 'en',
		dir: 'ltr',
		categories,
		icons: [
			{ src: icon192, sizes: '192x192', type: 'image/png', purpose: 'any' },
			{ src: icon512, sizes: '512x512', type: 'image/png', purpose: 'any' },
			{ src: maskable, sizes: '512x512', type: 'image/png', purpose: 'maskable' }
		]
	};

	if (shortcuts.length > 0) manifest.shortcuts = shortcuts;

	const shots = jsonArray(settings, 'pwa.screenshots').filter(
		(s): s is Record<string, unknown> => typeof s === 'object' && s !== null && typeof (s as Record<string, unknown>).src === 'string'
	);
	if (shots.length > 0) manifest.screenshots = shots;

	if (app.includeShareTarget && bool(settings, 'pwa.share_target_enabled', false)) {
		// Shared title/text/url prefill the /studio/create composer (which
		// /create 308-redirects to, preserving the query string).
		manifest.share_target = {
			action: '/create',
			method: 'GET',
			params: { title: 'title', text: 'text', url: 'url' }
		};
	}

	return manifest;
}

// ---------------------------------------------------------------------------
// Service worker — offline caching
// ---------------------------------------------------------------------------

export interface ServiceWorkerInit {
	/** Cache-busting build identifier from $service-worker (`version`). */
	version: string;
	/** Absolute URL for GET /site/settings/public on the API host. */
	settingsUrl?: string;
	/** Precached offline fallback document. Default '/offline.html'. */
	offlineUrl?: string;
	/** Extra URLs to precache at install (manifest icons, etc). */
	precacheExtras?: string[];
}

let pwaConfigCheckedAt = 0;
let offlineEnabled = true;
const CONFIG_REFRESH_MS = 5 * 60_000;

// Authenticated navigations are never written to the page cache — a stale
// personalized page must not outlive its session on shared devices. The SW
// can't read the Cookie header (forbidden name), so pages report their auth
// state via postMessage; default true (privacy-safe) until a page reports
// guest. Guests get page caching; authed users get network-only navigations.
let authed = true;

async function purgeAllPwaCaches(): Promise<void> {
	const names = await caches.keys();
	await Promise.all(names.filter((n) => n.startsWith('jc-')).map((n) => caches.delete(n)));
}

async function refreshPwaConfig(settingsUrl: string | undefined): Promise<void> {
	if (!settingsUrl) return;
	pwaConfigCheckedAt = Date.now();
	try {
		const res = await fetch(settingsUrl);
		if (!res.ok) return;
		const body = await res.json();
		const data = body?.data ?? body;
		const wasEnabled = offlineEnabled;
		offlineEnabled = !(data && data['pwa.offline_enabled'] === false);
		// A flag flip to disabled purges everything — "off" means no cached
		// residue either, not just "stop serving from cache".
		if (wasEnabled && !offlineEnabled) await purgeAllPwaCaches();
	} catch {
		// Offline or transient failure — keep the enabled default.
	}
}

function isCacheable(res: Response): boolean {
	// Skip error responses and anything the server asked intermediaries not
	// to store — a personalized response must never be served to a later
	// logged-out session offline. (Set-Cookie is a forbidden header name —
	// never visible to fetch consumers, so Cache-Control is the real gate.)
	if (!res.ok) return false;
	const cc = res.headers.get('cache-control') ?? '';
	return !/no-store|private/i.test(cc);
}

async function cacheFirst(event: FetchEvent, cacheName: string): Promise<Response> {
	const { request } = event;
	const cache = await caches.open(cacheName);
	const hit = await cache.match(request);
	if (hit) return hit;
	const res = await fetch(request);
	// waitUntil so the write lands even if the worker would otherwise be
	// terminated once respondWith's promise settles.
	if (isCacheable(res)) event.waitUntil(cache.put(request, res.clone()));
	return res;
}

async function networkFirstNavigation(
	event: FetchEvent,
	pagesCache: string,
	precache: string,
	offlineUrl: string
): Promise<Response> {
	const { request } = event;
	const cache = await caches.open(pagesCache);
	try {
		const res = await fetch(request);
		if (isCacheable(res) && !authed) event.waitUntil(cache.put(request, res.clone()));
		return res;
	} catch {
		const hit = await cache.match(request);
		if (hit) return hit;
		const offline = await caches.open(precache);
		const fallback = await offline.match(offlineUrl);
		return fallback ?? Response.error();
	}
}

async function staleWhileRevalidate(event: FetchEvent, cacheName: string): Promise<Response> {
	const { request } = event;
	const cache = await caches.open(cacheName);
	const hit = await cache.match(request);
	const fetched = fetch(request)
		.then(async (res) => {
			if (isCacheable(res)) await cache.put(request, res.clone());
			return res;
		})
		.catch(() => undefined);
	// Without waitUntil the worker can be killed mid-revalidation — the cached
	// copy would serve forever with the update never landing.
	event.waitUntil(fetched);
	return hit ?? (await fetched) ?? Response.error();
}

/**
 * Wire install/activate/fetch handlers for the PWA caching layer. The caller
 * still owns install/activate for its own purposes — this adds listeners, it
 * does not replace them (skipWaiting / clients.claim live in the app worker).
 */
export function initPwaServiceWorker(sw: ServiceWorkerGlobalScope, init: ServiceWorkerInit): void {
	const offlineUrl = init.offlineUrl ?? '/offline.html';
	const PRECACHE = `jc-precache-${init.version}`;
	const IMMUTABLE = `jc-immutable-${init.version}`;
	const PAGES = `jc-pages-${init.version}`;
	const RUNTIME = `jc-runtime-${init.version}`;
	const ACTIVE = new Set([PRECACHE, IMMUTABLE, PAGES, RUNTIME]);
	const precacheUrls = [offlineUrl, '/manifest.json', ...(init.precacheExtras ?? [])];

	sw.addEventListener('install', (event) => {
		event.waitUntil(
			(async () => {
				// Read the offline flag first — a disabled toggle must mean no
				// caching at all, not "cache but never serve".
				await refreshPwaConfig(init.settingsUrl);
				if (!offlineEnabled) return;
				const cache = await caches.open(PRECACHE);
				// Per-URL adds: one missing precache asset must not fail the whole
				// install — a failing addAll would brick the worker (and, on the
				// main app, take push notifications down with it).
				await Promise.allSettled(precacheUrls.map((url) => cache.add(url)));
			})()
		);
	});

	sw.addEventListener('activate', (event) => {
		event.waitUntil(
			(async () => {
				const names = await caches.keys();
				await Promise.all(
					names
						.filter((n) => n.startsWith('jc-') && !ACTIVE.has(n))
						.map((n) => caches.delete(n))
				);
				await refreshPwaConfig(init.settingsUrl);
			})()
		);
	});

	// Pages inform the worker of their auth state so guest navigations cache
	// and authed ones don't. Transitioning to authed purges the page cache —
	// anything cached while the previous state held is no longer safe to keep.
	sw.addEventListener('message', (event) => {
		const data = event.data as { type?: string; authed?: boolean } | undefined;
		if (data?.type !== 'jc:pwa-auth' || typeof data.authed !== 'boolean') return;
		const wasAuthed = authed;
		authed = data.authed;
		if (authed && !wasAuthed) event.waitUntil(caches.delete(PAGES));
	});

	sw.addEventListener('fetch', (event) => {
		const { request } = event;
		if (request.method !== 'GET' || !offlineEnabled) return;
		// Re-check the offline flag periodically — picks up admin toggles within
		// CONFIG_REFRESH_MS instead of once per worker lifetime, and retries
		// after a failed first fetch.
		if (Date.now() - pwaConfigCheckedAt > CONFIG_REFRESH_MS) {
			event.waitUntil(refreshPwaConfig(init.settingsUrl));
		}
		const url = new URL(request.url);
		if (url.origin !== sw.location.origin) return;
		// Same-origin /api/* routes are auth/session endpoints in the main app —
		// never serve them from cache.
		if (url.pathname.startsWith('/api/')) return;

		if (url.pathname.startsWith('/_app/immutable/')) {
			event.respondWith(cacheFirst(event, IMMUTABLE));
			return;
		}
		if (request.mode === 'navigate') {
			event.respondWith(networkFirstNavigation(event, PAGES, PRECACHE, offlineUrl));
			return;
		}
		event.respondWith(staleWhileRevalidate(event, RUNTIME));
	});
}

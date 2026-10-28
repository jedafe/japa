// Proxies /robots.txt to the Go backend's dynamic robots.txt handler.
//
// robots.txt is generated dynamically from the admin-configured bot registry
// (Section 43) and cached in Valkey. In production nginx routes this path
// straight to the Go API, so this route only serves dev and non-nginx
// deployments — but the two must not drift: the fallback below mirrors the
// backend's default disallow list, and the last good backend response is
// served stale before failing open so registry policies (e.g. blocked AI
// crawlers) survive a transient backend outage.
//
// Fails open deliberately: a hard Disallow: / on transient backend failure
// would deindex the site. The static fallback cannot honor blocked-bot
// policies — that's the accepted trade-off of this shadow path.
//
// Ref: japaconnect-engineering/sections/43-bot-crawler-management.md
import type { RequestHandler } from './$types';
import { env } from '$env/dynamic/private';
import { API_BASE, JC_SURFACE } from '$lib/server/api';

// Fails open: only explicitly non-production environments are blocked, so a
// misconfigured production deploy cannot accidentally deindex itself.
const NON_PROD_ENVS = new Set(['development', 'dev', 'staging', 'stage', 'test', 'preview', 'local']);
const blocksIndexing = NON_PROD_ENVS.has((env.PUBLIC_JC_ENV ?? env.JC_ENV ?? '').toLowerCase());

const DISALLOW_ALL = 'User-agent: *\nDisallow: /\n';

// Mirrors bots.defaultDisallowPaths in internal/bots/robots.go — keep in sync.
const DISALLOW_PATHS = [
	'/api/',
	'/admin/',
	'/content/',
	'/r$',
	'/r/',
	'/search',
	'/settings',
	'/messages',
	'/notifications',
	'/bookmarks',
	'/dashboard',
	'/create',
	'/digest',
	'/onboarding',
	'/unsubscribe',
	'/logout',
	'/login',
	'/signup',
	'/forgot-password',
	'/reset-password',
	'/verify-email',
	'/verify-totp',
	'/claim-ownership',
	'/verification-request',
	'/oauth',
	'/me$',
	'/me/',
	'/org$',
	'/org/',
	'/studio$',
	'/studio/',
	'/save$',
	'/save/'
];

const fallback = (origin: string) =>
	`User-agent: *\n${DISALLOW_PATHS.map((p) => `Disallow: ${p}`).join('\n')}\n\nSitemap: ${origin}/sitemap.xml\n`;

// Last successfully fetched backend response — served stale when the backend
// is unreachable so bot policies aren't silently dropped.
let lastGood: string | undefined;

export const GET: RequestHandler = async ({ request, url, getClientAddress }) => {
	// Non-production deployments are never crawlable, even if the backend is
	// unreachable — stage/dev must not leak into search indexes.
	if (blocksIndexing) {
		return new Response(DISALLOW_ALL, {
			headers: { 'Content-Type': 'text/plain; charset=utf-8', 'Cache-Control': 'public, max-age=300' }
		});
	}

	try {
		const backendURL = `${API_BASE}/robots.txt`;
		const headers: Record<string, string> = {
			'X-Forwarded-Host': url.hostname,
			// Identify this surface for scoped feature-flag resolution.
			'x-jc-surface': JC_SURFACE
		};
		// Forward the real client IP so the backend's GeoIP middleware can
		// resolve the visitor's country for geo-aware personalization (ADR-064).
		const clientIP = getClientAddress();
		if (clientIP) headers['x-forwarded-for'] = clientIP;
		const res = await fetch(backendURL, {
			headers,
			signal: request.signal
		});

		if (res.ok) {
			const text = await res.text();
			lastGood = text;
			return new Response(text, {
				headers: {
					'Content-Type': 'text/plain; charset=utf-8',
					'Cache-Control': 'public, max-age=86400'
				}
			});
		}
	} catch {
		// Backend unreachable — serve stale or the static fallback below.
	}

	const body = lastGood ?? fallback(url.origin);
	return new Response(body, {
		headers: {
			'Content-Type': 'text/plain; charset=utf-8',
			// Short cache on degraded responses so recovery is picked up fast.
			'Cache-Control': lastGood ? 'public, max-age=3600' : 'public, max-age=600'
		}
	});
};

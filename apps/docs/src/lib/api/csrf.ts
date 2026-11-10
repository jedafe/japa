// Double-submit CSRF for the Docs surface (§5.2). The token lives in the
// non-httpOnly csrf_token cookie — mirrored onto the browser by
// hooks.server.ts — so reads come from document.cookie and refreshes hit
// GET /csrf directly.
import { PUBLIC_JC_API_URL } from '$app/env/public';
import { buildApiUrl } from '@japaconnect/api-url';
import { getCookie } from '#lib/api/client.js';

export function getCsrfToken(): string | null {
	return getCookie('csrf_token') ?? null;
}

/**
 * Fetch a fresh CSRF token and return it. GET /csrf both sets the
 * csrf_token cookie (the double-submit half the middleware reads) and
 * echoes the token in the body — the body value is returned so callers
 * can attach it to a retry even when the cookie isn't readable from this
 * surface's document.cookie (e.g. a host-only cookie on the API host).
 */
export async function refreshCsrfToken(): Promise<string | null> {
	const base = (PUBLIC_JC_API_URL ?? '/api/v1').replace(/\/+$/, '');
	try {
		const res = await fetch(buildApiUrl('/csrf', base), { credentials: 'include' });
		const body = (await res.json()) as { data?: { csrfToken?: string } };
		return body?.data?.csrfToken ?? null;
	} catch {
		return null;
	}
}

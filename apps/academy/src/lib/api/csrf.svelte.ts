// Ref: japa-bugs-hunters.md §5.2 (CSRF) — audit finding C1.
// Browser-side double-submit CSRF token store. The backend issues a
// csrf_token cookie (non-HttpOnly) and echoes the same value from
// GET /api/v1/csrf as { data: { csrfToken } }. Mutating client requests
// must send that value as the X-CSRF-Token header so the backend's
// csrf.Middleware accepts them.
//
// The token is seeded from $page.data.csrfToken (fetched server-side in
// hooks.server.ts) on first hydration, then refreshed from /csrf whenever a
// 403 csrf_token_invalid is received. This avoids a boot-time fetch round-trip
// for authenticated users.

import { PUBLIC_JC_API_URL } from '$env/static/public';
import { buildApiUrl } from '@japaconnect/api-url';

let token: string | null = $state(null);
let fetching: Promise<string | null> | null = null;

/** Seed the token from server-rendered page data (no extra fetch needed). */
export function seedCsrfToken(value: string | null): void {
	if (value && token !== value) token = value;
}

/** Current CSRF token, or null if none has been acquired yet. */
export function getCsrfToken(): string | null {
	return token;
}

/**
 * Fetch a fresh CSRF token from GET /csrf. Cached so concurrent callers share
 * a single in-flight request. Returns null on failure (the caller will then
 * send no header and the backend will reject with 403, which triggers a retry).
 */
export async function refreshCsrfToken(): Promise<string | null> {
	if (fetching) return fetching;
	fetching = (async () => {
		try {
			const res = await fetch(buildApiUrl('/csrf', PUBLIC_JC_API_URL), {
				credentials: 'include',
				headers: { 'x-jc-surface': 'academy' }
			});
			if (!res.ok) return token;
			const body = (await res.json()) as { data?: { csrfToken?: string } };
			const next = body?.data?.csrfToken ?? null;
			if (next) token = next;
			return token;
		} catch {
			return token;
		} finally {
			fetching = null;
		}
	})();
	return fetching;
}

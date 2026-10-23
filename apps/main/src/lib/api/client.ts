import { PUBLIC_JC_API_URL } from '$env/static/public';
import { page } from '$app/state';
import { untrack } from 'svelte';
import { getCsrfToken, refreshCsrfToken } from '$lib/api/csrf.svelte';
import { getAccessToken, seedAccessToken, clearAccessToken } from '$lib/api/token.svelte';
import { buildApiUrl } from '@japaconnect/api-url';
import type { ListMeta } from '$lib/types';

/**
 * Browser-side API client for mutations and client-initiated reads.
 * Requests go to PUBLIC_JC_API_URL (proxied to the backend in dev). The proxy
 * converts the jc_token cookie into a Bearer header and forwards the request host,
 * so callers only pass path + body.
 *
 * Mutating requests (POST/PATCH/PUT/DELETE) attach the X-CSRF-Token header from
 * the double-submit CSRF token store (Ref: §5.2 — audit finding C1). On a 403
 * csrf_token_invalid the token is refreshed once and the request retried.
 */

export interface ClientResult<T> {
	data: T | null;
	error: { code: string; message: string; status: number; data?: Record<string, unknown> } | null;
	status: number;
}

/** Result including sidecar envelope fields (e.g. deleted_user_ids for GDPR). */
export interface ClientEnvelopeResult<T> extends ClientResult<T> {
	/** Raw envelope body (data + sidecar fields like deleted_user_ids). */
	envelope: Record<string, unknown> | null;
}

const API_BASE = (PUBLIC_JC_API_URL ?? '/api/v1').replace(/\/+$/, '');

export const apiBaseUrl = API_BASE;

/**
 * Surface identifier sent on every backend call (X-JC-Surface). The backend
 * resolves feature flags and geo rules per surface — the valid values are
 * defined in internal/features/eval.go (ValidSurface).
 */
const JC_SURFACE = 'main';

const MUTATING_METHODS = new Set(['POST', 'PATCH', 'PUT', 'DELETE']);

/** In-flight browser refresh; shared so concurrent 401s coalesce into one call. */
let refreshPromise: Promise<boolean> | null = null;

// A successful refresh rotates the one-time-use jc_refresh cookie. A second
// refresh fired before the browser has applied the Set-Cookie would present
// the just-revoked cookie, trip the server's reuse detection, and revoke the
// entire session family. A refresh that succeeded within this window is
// treated as still valid — the retrying request only needs the rotated
// jc_token cookie, not another refresh-cookie rotation.
const REFRESH_OK_TTL_MS = 3_000;
let lastRefreshOkAt = 0;

/**
 * Refresh the access token via the SvelteKit /api/auth/refresh endpoint.
 * The backend rotates the refresh token and the endpoint sets both jc_token
 * and jc_refresh cookies, so the next proxied request will carry the new
 * access token automatically. Returns true on success.
 */
async function refreshSession(): Promise<boolean> {
	if (refreshPromise) return refreshPromise;
	if (Date.now() - lastRefreshOkAt < REFRESH_OK_TTL_MS) return true;
	refreshPromise = (async () => {
		try {
			const res = await fetch('/api/auth/refresh', {
				method: 'POST',
				credentials: 'include'
			});
			if (!res.ok) return false;
			lastRefreshOkAt = Date.now();
			// The endpoint returns the freshly minted access token in the body —
			// seed the in-memory store so retried requests carry the new Bearer.
			try {
				const body = (await res.clone().json()) as { access_token?: string };
				if (body?.access_token) seedAccessToken(body.access_token);
			} catch {
				// Body absent/unparseable — the cookie was still rotated; the
				// retried request will 401 again and re-enter refresh.
			}
			return true;
		} catch {
			return false;
		} finally {
			refreshPromise = null;
		}
	})();
	return refreshPromise;
}

function withCsrfHeader(method: string, headers: Record<string, string>): Record<string, string> {
	// untrack: credential stores are $state, and a synchronous read inside a
	// caller's $effect would subscribe the effect to token rotation — a
	// seed/refresh then re-fires the effect (duplicate fetches).
	const token = untrack(getCsrfToken);
	if (MUTATING_METHODS.has(method.toUpperCase()) && token) {
		return { ...headers, 'x-csrf-token': token };
	}
	return headers;
}

async function clientFetch<T>(
	path: string,
	init: RequestInit = {}
): Promise<ClientResult<T>> {
	const r = await clientFetchRaw<T>(path, init);
	return { data: r.data, error: r.error, status: r.status };
}

/** Internal: clientFetch that also returns the raw parsed envelope body. */
async function clientFetchRaw<T>(
	path: string,
	init: RequestInit = {}
): Promise<ClientEnvelopeResult<T>> {
	const method = (init.method ?? 'GET').toString().toUpperCase();
	const isFormData = init.body instanceof FormData;
	const baseHeaders: Record<string, string> = {
		// For FormData, let the browser set the multipart boundary.
		...(isFormData ? {} : { 'content-type': 'application/json' }),
		// Identify this surface for scoped feature-flag resolution.
		'x-jc-surface': JC_SURFACE,
		...((init.headers as Record<string, string>) ?? {})
	};

	// The Go API authenticates Authorization: Bearer only — the HttpOnly
	// jc_token cookie never reaches api.<domain> and cannot be read by JS
	// anyway. The in-memory token (seeded from SSR page data / refresh)
	// is the browser's credential. Bearer also makes the request CSRF-exempt
	// (IsNonCookieAuthenticated) since cross-site callers can't forge headers.
	const accessToken = untrack(getAccessToken);
	if (accessToken && !('authorization' in baseHeaders) && !('Authorization' in baseHeaders)) {
		baseHeaders['authorization'] = `Bearer ${accessToken}`;
	}

	async function doFetch(): Promise<Response> {
		return fetch(buildApiUrl(path, API_BASE), {
			...init,
			credentials: 'include',
			headers: withCsrfHeader(method, baseHeaders)
		});
	}

	try {
		let res = await doFetch();

		// On an expired/revoked access token, try to refresh once and retry.
		if (res.status === 401) {
			let errBody: { error?: { code?: string } } | null = null;
			try {
				errBody = (await res.clone().json()) as { error?: { code?: string } };
			} catch {
				errBody = null;
			}
			const code = errBody?.error?.code;
			// An expired/missing/revoked access token may be recoverable with
			// the HttpOnly jc_refresh cookie. Don't retry credentials errors.
			if (code === 'missing_token' || code === 'invalid_token' || code === 'token_revoked') {
				const refreshed = await refreshSession();
				if (refreshed) {
					res = await doFetch();
				} else if (page.data?.isAuthed === true) {
					// The session is unrecoverable — the refresh token was
					// revoked or expired (password change, logout on a sibling
					// surface, or family revocation per §02). Reloading lets
					// hooks.server.ts clear the dead cookies and render the
					// guest view; authed-only loads (e.g. /settings/*) redirect
					// to /login server-side. Guests never reach this branch —
					// without the isAuthed gate a guest hitting an authed
					// endpoint would reload-loop.
					clearAccessToken();
					window.location.reload();
				}
			}
		}

		// On a CSRF rejection, refresh the token once and retry the same request.
		if (res.status === 403 && MUTATING_METHODS.has(method)) {
			let errBody: { error?: { code?: string } } | null = null;
			try {
				errBody = (await res.clone().json()) as { error?: { code?: string } };
			} catch {
				errBody = null;
			}
			if (errBody?.error?.code === 'csrf_token_invalid') {
				await refreshCsrfToken();
				res = await doFetch();
			}
		}

		let body: unknown = null;
		// 204 No Content carries no body — res.json() would throw on the
		// empty stream, so data stays null (callers treat null as success
		// with no payload, e.g. wiki/jce delete paths).
		if (res.status !== 204) {
			// Sniff the content type: text/plain (or empty) bodies that
			// carry JSON (proxies/health shims) still decode; anything else
			// yields null instead of a json() throw.
			const contentType = res.headers.get('content-type') ?? '';
			if (contentType.includes('json') || contentType.includes('text') || contentType === '') {
				try {
					const text = await res.text();
					body = text.trim() === '' ? null : (JSON.parse(text) as unknown);
				} catch {
					body = null;
				}
			}
		}

		if (!res.ok || (body && typeof body === 'object' && 'error' in body)) {
			const err = (body as { error?: { code?: string; message?: string; data?: Record<string, unknown> } } | null)?.error;
			return {
				data: null,
				envelope: null,
				error: {
					code: err?.code ?? 'request_failed',
					message: err?.message ?? res.statusText,
					status: res.status,
					data: err?.data
				},
				status: res.status
			};
		}

		return {
			data: (body as { data: T } | null)?.data ?? null,
			envelope: (body as Record<string, unknown> | null) ?? null,
			error: null,
			status: res.status
		};
	} catch (e) {
		return {
			data: null,
			envelope: null,
			error: {
				code: 'network_error',
				message: e instanceof Error ? e.message : 'Network error',
				status: 0
			},
			status: 0
		};
	}
}

export const api = {
	get: <T>(path: string) => clientFetch<T>(path, { method: 'GET' }),
	/** Like get() but also returns the raw envelope (sidecar fields like deleted_user_ids). */
	getEnvelope: <T>(path: string): Promise<ClientEnvelopeResult<T>> =>
		clientFetchRaw<T>(path, { method: 'GET' }),
	post: <T>(path: string, body?: unknown, headers?: Record<string, string>) =>
		clientFetch<T>(path, {
			method: 'POST',
			body: body ? JSON.stringify(body) : undefined,
			headers
		}),
	postMultipart: <T>(path: string, formData: FormData) =>
		clientFetch<T>(path, {
			method: 'POST',
			body: formData
		}),
	patch: <T>(path: string, body?: unknown, headers?: Record<string, string>) =>
		clientFetch<T>(path, {
			method: 'PATCH',
			body: body ? JSON.stringify(body) : undefined,
			headers
		}),
	put: <T>(path: string, body?: unknown, headers?: Record<string, string>) =>
		clientFetch<T>(path, {
			method: 'PUT',
			body: body ? JSON.stringify(body) : undefined,
			headers
		}),
	delete: <T>(path: string, body?: unknown, headers?: Record<string, string>) =>
		clientFetch<T>(path, {
			method: 'DELETE',
			body: body ? JSON.stringify(body) : undefined,
			headers
		})
};

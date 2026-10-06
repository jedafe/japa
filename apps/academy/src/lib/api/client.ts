import { PUBLIC_JC_API_URL } from '$env/static/public';
import { getCsrfToken, refreshCsrfToken } from '$lib/api/csrf.svelte';
import { buildApiUrl } from '@japaconnect/api-url';
import { loginUrl } from '$lib/appOrigins';

/**
 * Browser-side API client for mutations and client-initiated reads.
 * Requests go to PUBLIC_JC_API_URL (proxied to the backend in dev). The proxy
 * converts the jc_token cookie into a Bearer header and forwards the request host,
 * so callers only pass path + body.
 *
 * Mutating requests (POST/PATCH/PUT/DELETE) attach the X-CSRF-Token header from
 * the double-submit CSRF token store (Ref: §5.2 — audit finding C1). On a 403
 * csrf_token_invalid the token is refreshed once and the request retried.
 *
 * CRITICAL (ADR-060 SSR/CSR boundary): user-specific data (academy_enrollments, progress,
 * quiz scores, certificates) is fetched client-side via this client ONLY — never
 * in +page.server.ts load() functions. See the skill's SSR/CSR boundary rule.
 */

export interface ClientResult<T> {
	data: T | null;
	error: { code: string; message: string; status: number } | null;
	status: number;
}

const API_BASE = (PUBLIC_JC_API_URL ?? '/api/v1').replace(/\/+$/, '');

const MUTATING_METHODS = new Set(['POST', 'PATCH', 'PUT', 'DELETE']);

// Guard against repeated 401 redirects — once a redirect to login has been
// triggered, subsequent 401s in the same page lifecycle return immediately
// without triggering another navigation (prevents a redirect loop).
let sessionRedirecting = false;

// Whether the current browser tab holds an authenticated session. The
// double-submit csrf_token cookie is set by hooks.server.ts ONLY for
// authenticated sessions (httpOnly:false, so it is readable here). Its
// presence is a safe proxy for "has session" — guests never receive it, so a
// 403 read carrying this cookie is a genuinely-forbidden authed request.
export function clientHasSession(): boolean {
	if (typeof document === 'undefined') return false;
	return document.cookie
		.split(';')
		.some((c) => c.trim().startsWith('csrf_token='));
}

function withCsrfHeader(method: string, headers: Record<string, string>): Record<string, string> {
	const token = getCsrfToken();
	if (MUTATING_METHODS.has(method.toUpperCase()) && token) {
		return { ...headers, 'x-csrf-token': token };
	}
	return headers;
}

async function clientFetch<T>(
	path: string,
	init: RequestInit = {},
	extractData = true
): Promise<ClientResult<T>> {
	const method = (init.method ?? 'GET').toString().toUpperCase();
	const baseHeaders: Record<string, string> = {
		'content-type': 'application/json',
		...((init.headers as Record<string, string>) ?? {})
	};

	async function doFetch(): Promise<Response> {
		return fetch(buildApiUrl(path, API_BASE), {
			...init,
			credentials: 'include',
			headers: withCsrfHeader(method, baseHeaders)
		});
	}

	try {
		let res = await doFetch();

		// On 401 (session expired) OR a 403 on an unauthenticated READ, redirect
		// to the Main app login page once. The redirect guard prevents an
		// infinite loop if the login round-trip still yields 401 (e.g. token
		// revoked). Uses window.location (not goto) because login is
		// cross-origin on the Main app (ADR-060).
		//
		// 403 handling (systemic auth-redirect gap fix): a 403 on a *mutating*
		// request is owned by the CSRF-retry block below and must NOT redirect
		// to login. A 403 on a READ request that carries no session is an
		// "unauthenticated" 403 — in the public-by-default app a guest hitting a
		// protected read must be sent to login rather than shown empty/misleading
		// UI. A 403 on a READ that DOES carry a session is a genuinely-forbidden
		// authed request and is left alone. Behaviour for 401 is unchanged.
		if (
			(res.status === 401 ||
				(res.status === 403 && !MUTATING_METHODS.has(method) && !clientHasSession())) &&
			!sessionRedirecting
		) {
			sessionRedirecting = true;
			const returnTo = window.location.pathname + window.location.search;
			window.location.href = loginUrl(returnTo);
			return {
				data: null,
				error: { code: 'unauthorized', message: 'Session expired', status: res.status },
				status: res.status
			};
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
		try {
			body = await res.json();
		} catch {
			body = null;
		}

		if (!res.ok || (body && typeof body === 'object' && 'error' in body)) {
			const err = (body as { error?: { code?: string; message?: string } } | null)?.error;
			return {
				data: null,
				error: {
					code: err?.code ?? 'request_failed',
					message: err?.message ?? res.statusText,
					status: res.status
				},
				status: res.status
			};
		}

		const resultData = extractData
			? (body as { data: T } | null)?.data ?? null
			: (body as T | null) ?? null;

		return {
			data: resultData,
			error: null,
			status: res.status
		};
	} catch (e) {
		return {
			data: null,
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
	get: <T>(path: string, headers?: Record<string, string>) =>
		clientFetch<T>(path, { method: 'GET', headers }),
	// Raw body (no `data` envelope extraction) — for endpoints that return
	// sibling top-level fields such as `aggregate` alongside `data`.
	getBody: <T>(path: string, headers?: Record<string, string>) =>
		clientFetch<T>(path, { method: 'GET', headers }, false),
	post: <T>(path: string, body?: unknown, headers?: Record<string, string>) =>
		clientFetch<T>(path, {
			method: 'POST',
			body: body ? JSON.stringify(body) : undefined,
			headers
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
	delete: <T>(path: string, headers?: Record<string, string>) =>
		clientFetch<T>(path, { method: 'DELETE', headers })
};

import { PUBLIC_JC_API_URL } from '$env/static/public';
import { getCsrfToken, refreshCsrfToken } from '$lib/api/csrf.svelte';
import { buildApiUrl } from '@japaconnect/api-url';

/**
 * Browser-side API client for mutations and client-initiated reads.
 * Requests go to PUBLIC_JC_API_URL (proxied to the backend in dev). The proxy
 * converts the jc_token cookie into a Bearer header and forwards the tenant host,
 * so callers only pass path + body.
 *
 * Mutating requests (POST/PATCH/PUT/DELETE) attach the X-CSRF-Token header from
 * the double-submit CSRF token store (Ref: §5.2 — audit finding C1). On a 403
 * csrf_token_invalid the token is refreshed once and the request retried.
 */

export interface ClientResult<T> {
	data: T | null;
	error: { code: string; message: string; status: number } | null;
	status: number;
}

/** Result including sidecar envelope fields (e.g. deleted_user_ids for GDPR). */
export interface ClientEnvelopeResult<T> extends ClientResult<T> {
	/** Raw envelope body (data + sidecar fields like deleted_user_ids). */
	envelope: Record<string, unknown> | null;
}

const API_BASE = (PUBLIC_JC_API_URL ?? '/api/v1').replace(/\/+$/, '');

export const apiBaseUrl = API_BASE;

const MUTATING_METHODS = new Set(['POST', 'PATCH', 'PUT', 'DELETE']);

/** In-flight browser refresh; shared so concurrent 401s coalesce into one call. */
let refreshPromise: Promise<boolean> | null = null;

/**
 * Refresh the access token via the SvelteKit /api/auth/refresh endpoint.
 * The backend rotates the refresh token and the endpoint sets both jc_token
 * and jc_refresh cookies, so the next proxied request will carry the new
 * access token automatically. Returns true on success.
 */
async function refreshSession(): Promise<boolean> {
	if (refreshPromise) return refreshPromise;
	refreshPromise = (async () => {
		try {
			const res = await fetch('/api/auth/refresh', {
				method: 'POST',
				credentials: 'include'
			});
			return res.ok;
		} catch {
			return false;
		} finally {
			refreshPromise = null;
		}
	})();
	return refreshPromise;
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
		try {
			body = await res.json();
		} catch {
			body = null;
		}

		if (!res.ok || (body && typeof body === 'object' && 'error' in body)) {
			const err = (body as { error?: { code?: string; message?: string } } | null)?.error;
			return {
				data: null,
				envelope: null,
				error: {
					code: err?.code ?? 'request_failed',
					message: err?.message ?? res.statusText,
					status: res.status
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

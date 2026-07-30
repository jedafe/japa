import { PUBLIC_JC_API_URL } from '$env/static/public';
import { getCsrfToken, refreshCsrfToken } from '$lib/api/csrf.svelte';

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

const MUTATING_METHODS = new Set(['POST', 'PATCH', 'PUT', 'DELETE']);

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
	const method = (init.method ?? 'GET').toString().toUpperCase();
	const baseHeaders: Record<string, string> = {
		'content-type': 'application/json',
		...((init.headers as Record<string, string>) ?? {})
	};

	async function doFetch(): Promise<Response> {
		return fetch(`${PUBLIC_JC_API_URL}${path}`, {
			...init,
			credentials: 'include',
			headers: withCsrfHeader(method, baseHeaders)
		});
	}

	try {
		let res = await doFetch();

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

		return {
			data: (body as { data: T } | null)?.data ?? null,
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
	get: <T>(path: string) => clientFetch<T>(path, { method: 'GET' }),
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
	delete: <T>(path: string, body?: unknown, headers?: Record<string, string>) =>
		clientFetch<T>(path, {
			method: 'DELETE',
			body: body ? JSON.stringify(body) : undefined,
			headers
		})
};

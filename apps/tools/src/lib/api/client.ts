import { PUBLIC_JC_API_URL } from '$env/static/public';
import { getCsrfToken, refreshCsrfToken } from '$lib/api/csrf.svelte';
import { buildApiUrl } from '@japaconnect/api-url';
import type { ListMeta } from '$lib/types';

/**
 * Browser-side API client for mutations and client-initiated reads.
 * Requests go to PUBLIC_JC_API_URL (proxied to the backend in dev). The proxy
 * converts the jc_token cookie into a Bearer header and forwards the request host,
 * so callers only pass path + body.
 *
 * Mutating requests (POST/PATCH/PUT/DELETE) attach the X-CSRF-Token header from
 * the double-submit CSRF token store (Ref: §5.2). On a 403 csrf_token_invalid
 * the token is refreshed once and the request retried.
 *
 * Tools surface (ADR-086): user-specific data (checklist progress, assessment
 * results, saved scholarships, remittance alerts) is fetched client-side via
 * this client when the interaction is interactive (tick, save, alert create).
 * SSR load() functions fetch the initial page data server-side.
 */

export interface ClientResult<T> {
	data: T | null;
	meta?: ListMeta;
	error: { code: string; message: string; status: number } | null;
	status: number;
}

const API_BASE = (PUBLIC_JC_API_URL ?? '/api/v1').replace(/\/+$/, '');

/**
 * Surface identifier sent on every backend call (X-JC-Surface). The backend
 * resolves feature flags and geo rules per surface — the valid values are
 * defined in internal/features/eval.go (ValidSurface).
 */
const JC_SURFACE = 'tools';

const MUTATING_METHODS = new Set(['POST', 'PATCH', 'PUT', 'DELETE']);

function withCsrfHeader(method: string, headers: Record<string, string>): Record<string, string> {
	const token = getCsrfToken();
	if (MUTATING_METHODS.has(method.toUpperCase()) && token) {
		return { ...headers, 'x-csrf-token': token };
	}
	return headers;
}

async function clientFetch<T>(path: string, init: RequestInit = {}): Promise<ClientResult<T>> {
	const method = (init.method ?? 'GET').toString().toUpperCase();
	const baseHeaders: Record<string, string> = {
		'content-type': 'application/json',
		// Identify this surface for scoped feature-flag resolution.
		'x-jc-surface': JC_SURFACE,
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

		// On a CSRF rejection, refresh the token once and retry the same request.
		if (res.status === 403 && MUTATING_METHODS.has(method)) {
			let errBody: { error?: { code?: string } } | null = null;
			try {
				errBody = await res.json();
			} catch {
				errBody = null;
			}
			if (errBody?.error?.code === 'csrf_token_invalid' || errBody?.error?.code === 'csrf_token_missing') {
				const refreshed = await refreshCsrfToken();
				if (refreshed) {
					res = await doFetch();
				}
			}
		}

		let body: unknown = null;
		try {
			body = await res.json();
		} catch {
			body = null;
		}

		const isErrorEnvelope = body !== null && typeof body === 'object' && 'error' in body;

		if (!res.ok || isErrorEnvelope) {
			const err = (body as ApiErr | null)?.error;
			return {
				data: null,
				error: {
					code: err?.code ?? 'api_error',
					message: err?.message ?? res.statusText,
					status: res.status
				},
				status: res.status
			};
		}

		const ok = body as ApiOk<T> & {
			pagination?: { total?: number; page?: number; limit?: number; page_size?: number };
		};
		// Some endpoints return pagination totals under a top-level `pagination`
		// key rather than `meta` (e.g. GET /attestation) — normalize to `meta`.
		const meta: ListMeta | undefined =
			ok.meta ??
			(ok.pagination?.total !== undefined
				? {
						total: ok.pagination.total,
						page: ok.pagination.page,
						limit: ok.pagination.limit ?? ok.pagination.page_size
					}
				: undefined);
		return { data: ok.data ?? (body as T), meta, error: null, status: res.status };
	} catch (err) {
		return {
			data: null,
			error: {
				code: 'network_error',
				message: err instanceof Error ? err.message : 'Network request failed',
				status: 0
			},
			status: 0
		};
	}
}

/** Convenience method wrappers — return the full ClientResult for error handling. */
export const api = {
	get: <T>(path: string, init?: RequestInit) => clientFetch<T>(path, { ...init, method: 'GET' }),
	post: <T>(path: string, body?: unknown, init?: RequestInit) =>
		clientFetch<T>(path, { ...init, method: 'POST', body: JSON.stringify(body) }),
	patch: <T>(path: string, body?: unknown, init?: RequestInit) =>
		clientFetch<T>(path, { ...init, method: 'PATCH', body: JSON.stringify(body) }),
	delete: <T>(path: string, init?: RequestInit) =>
		clientFetch<T>(path, { ...init, method: 'DELETE' })
};

import type { ApiErr, ApiOk } from '$lib/types';

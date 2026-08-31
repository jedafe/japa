import { PUBLIC_JC_API_URL } from '$env/static/public';
import { getCsrfToken, refreshCsrfToken } from '$lib/api/csrf.svelte';
import { buildApiUrl } from '@japaconnect/api-url';
import type { ApiErr, ApiOk } from '$lib/types';

export interface ClientResult<T> {
	data: T | null;
	error: { code: string; message: string; status: number } | null;
	status: number;
}

export const API_BASE = (PUBLIC_JC_API_URL ?? '/api/v1').replace(/\/+$/, '');

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

		const ok = body as ApiOk<T>;
		return { data: ok.data ?? (body as T), error: null, status: res.status };
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

export const api = {
	get: <T>(path: string, init?: RequestInit) => clientFetch<T>(path, { ...init, method: 'GET' }),
	post: <T>(path: string, body?: unknown, init?: RequestInit) =>
		clientFetch<T>(path, { ...init, method: 'POST', body: JSON.stringify(body) }),
	patch: <T>(path: string, body?: unknown, init?: RequestInit) =>
		clientFetch<T>(path, { ...init, method: 'PATCH', body: JSON.stringify(body) }),
	delete: <T>(path: string, init?: RequestInit) =>
		clientFetch<T>(path, { ...init, method: 'DELETE' })
};

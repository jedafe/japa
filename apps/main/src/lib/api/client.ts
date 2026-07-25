import { PUBLIC_JC_API_URL } from '$env/static/public';

/**
 * Browser-side API client for mutations and client-initiated reads.
 * Requests go to PUBLIC_JC_API_URL (proxied to the backend in dev). The proxy
 * converts the jc_token cookie into a Bearer header and forwards the tenant host,
 * so callers only pass path + body.
 */

export interface ClientResult<T> {
	data: T | null;
	error: { code: string; message: string; status: number } | null;
	status: number;
}

async function clientFetch<T>(
	path: string,
	init: RequestInit = {}
): Promise<ClientResult<T>> {
	try {
		const res = await fetch(`${PUBLIC_JC_API_URL}${path}`, {
			...init,
			credentials: 'include',
			headers: {
				'content-type': 'application/json',
				...init.headers
			}
		});

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
	post: <T>(path: string, body?: unknown) =>
		clientFetch<T>(path, { method: 'POST', body: body ? JSON.stringify(body) : undefined }),
	patch: <T>(path: string, body?: unknown) =>
		clientFetch<T>(path, { method: 'PATCH', body: body ? JSON.stringify(body) : undefined }),
	put: <T>(path: string, body?: unknown) =>
		clientFetch<T>(path, { method: 'PUT', body: body ? JSON.stringify(body) : undefined }),
	delete: <T>(path: string) => clientFetch<T>(path, { method: 'DELETE' })
};

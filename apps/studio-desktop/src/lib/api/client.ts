/**
 * Minimal JapaConnect API client for the desktop app.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

import { getAccessToken } from './auth';

export interface ApiResponse<T> {
	ok: boolean;
	data?: T;
	error?: { code: string; message: string };
}

export interface JapaEnvelope<D = unknown> {
	data: D;
	meta?: Record<string, unknown>;
}

export function createClient(baseUrl: string) {
	async function request<T>(path: string, options: RequestInit = {}): Promise<ApiResponse<T>> {
		const token = await getAccessToken();
		const url = `${baseUrl.replace(/\/$/, '')}/api/v1${path}`;

		const res = await fetch(url, {
			...options,
			headers: {
				'Content-Type': 'application/json',
				...(token ? { Authorization: `Bearer ${token}` } : {}),
				...options.headers
			}
		});

		let body: unknown = {};
		try {
			body = await res.json();
		} catch {
			body = {};
		}

		if (!res.ok) {
			const envelope = body as { error?: { code: string; message: string } };
			return {
				ok: false,
				error: envelope.error ?? { code: 'unknown_error', message: res.statusText }
			};
		}

		const envelope = body as { data?: T };
		return { ok: true, data: envelope.data as T };
	}

	return {
		get: <T>(path: string) => request<T>(path),
		post: <T>(path: string, body: unknown) => request<T>(path, { method: 'POST', body: JSON.stringify(body) }),
		patch: <T>(path: string, body: unknown) => request<T>(path, { method: 'PATCH', body: JSON.stringify(body) }),
		delete: <T>(path: string) => request<T>(path, { method: 'DELETE' })
	};
}

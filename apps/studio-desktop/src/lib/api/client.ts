/**
 * Minimal JapaConnect API client for the desktop app.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

export interface ApiResponse<T> {
	ok: boolean;
	data?: T;
	error?: { code: string; message: string };
}

export function createClient(baseUrl: string, apiKey: string) {
	async function request<T>(path: string, options: RequestInit = {}): Promise<ApiResponse<T>> {
		const url = `${baseUrl.replace(/\/$/, '')}/api/v1${path}`;
		const res = await fetch(url, {
			...options,
			headers: {
				'Content-Type': 'application/json',
				Authorization: `Bearer ${apiKey}`,
				...options.headers
			}
		});

		if (!res.ok) {
			const body = await res.json().catch(() => ({}));
			return {
				ok: false,
				error: body.error ?? { code: 'unknown_error', message: res.statusText }
			};
		}

		const data = (await res.json().catch(() => ({}))) as T;
		return { ok: true, data };
	}

	return {
		get: <T>(path: string) => request<T>(path),
		post: <T>(path: string, body: unknown) => request<T>(path, { method: 'POST', body: JSON.stringify(body) }),
		patch: <T>(path: string, body: unknown) => request<T>(path, { method: 'PATCH', body: JSON.stringify(body) }),
		delete: <T>(path: string) => request<T>(path, { method: 'DELETE' })
	};
}

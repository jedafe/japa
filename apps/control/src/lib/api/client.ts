import { PUBLIC_JC_API_URL } from '$env/static/public';

export interface ApiOk<T> {
	data: T;
}

export interface ApiErr {
	error: { code: string; message: string; status: number };
}

export interface ApiResult<T> {
	data: T | null;
	error: ApiErr['error'] | null;
	status: number;
}

export async function apiFetch<T>(
	path: string,
	options: RequestInit = {},
	fetchFn?: typeof fetch
): Promise<ApiResult<T>> {
	const fetcher = fetchFn ?? fetch;
	// Ref: internal/server/server.go — Control Panel routes are mounted at the
	// top-level /control/* group (NOT under /api/v1). The Vite dev proxy maps
	// /control -> http://localhost:8080, so pages call these endpoints as
	// /control/... and the request reaches the backend unchanged. Do not prefix
	// with /api/v1 here: that path is not registered and returns 404.
	const url = `${PUBLIC_JC_API_URL}${path}`;

	try {
		const headers: Record<string, string> = {
			...(options.headers as Record<string, string> | undefined),
		};
		if (options.body) {
			headers['Content-Type'] = 'application/json';
		}

		const res = await fetcher(url, {
			...options,
			credentials: 'include',
			headers,
		});

		const ct = res.headers.get('content-type') ?? '';
		let body: ApiOk<T> | ApiErr | null = null;
		if (ct.includes('application/json')) {
			body = (await res.json().catch(() => null)) as ApiOk<T> | ApiErr | null;
		} else if (res.status !== 204) {
			const text = await res.text();
			if (!res.ok) {
				return {
					data: null,
					error: { code: 'non_json_response', message: text.slice(0, 200) || res.statusText, status: res.status },
					status: res.status
				};
			}
		}

		// Auth failures: surface the backend's real error message (R5 envelope)
		// instead of a hardcoded string, so e.g. a failed login shows the actual
		// reason rather than a misleading "Session expired".
		if (res.status === 401) {
			const msg = (body && 'error' in body && body.error.message) || 'Session expired';
			return { data: null, error: { code: 'unauthorized', message: msg, status: 401 }, status: 401 };
		}

		if (res.status === 403) {
			const msg = (body && 'error' in body && body.error.message) || 'Insufficient privileges';
			return { data: null, error: { code: 'forbidden', message: msg, status: 403 }, status: 403 };
		}

		if (!res.ok || (body && 'error' in body)) {
			const err = body && 'error' in body ? body.error : { code: 'unknown', message: res.statusText, status: res.status };
			return { data: null, error: err, status: res.status };
		}

		return { data: (body as ApiOk<T>).data, error: null, status: res.status };
	} catch (err) {
		return {
			data: null,
			error: { code: 'network_error', message: err instanceof Error ? err.message : 'Network error', status: 0 },
			status: 0,
		};
	}
}

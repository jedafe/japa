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

let refreshPromise: Promise<boolean> | null = null;

async function attemptRefresh(): Promise<boolean> {
	if (refreshPromise) return refreshPromise;
	refreshPromise = (async () => {
		try {
			const res = await fetch(`${PUBLIC_JC_API_URL}/api/v1/auth/refresh`, {
				method: 'POST',
				credentials: 'include',
				headers: { 'Content-Type': 'application/json' },
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

export async function apiFetch<T>(
	path: string,
	options: RequestInit = {},
	fetchFn?: typeof fetch
): Promise<ApiResult<T>> {
	const fetcher = fetchFn ?? fetch;
	const url = `${PUBLIC_JC_API_URL}/api/v1${path}`;

	try {
		const res = await fetcher(url, {
			...options,
			credentials: 'include',
			headers: {
				'Content-Type': 'application/json',
				...options.headers,
			},
		});

		if (res.status === 401) {
			const refreshed = await attemptRefresh();
			if (refreshed) {
				const retryRes = await fetcher(url, {
					...options,
					credentials: 'include',
					headers: {
						'Content-Type': 'application/json',
						...options.headers,
					},
				});
				if (retryRes.ok) {
					const body = await retryRes.json();
					return { data: (body as ApiOk<T>).data, error: null, status: retryRes.status };
				}
			}
			return { data: null, error: { code: 'unauthorized', message: 'Session expired', status: 401 }, status: 401 };
		}

		if (res.status === 204 || res.status === 205 || res.headers.get('content-length') === '0') {
			return { data: null, error: null, status: res.status };
		}

		const body = await res.json();

		if (!res.ok || 'error' in body) {
			const err = 'error' in body ? body.error : { code: 'unknown', message: res.statusText, status: res.status };
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

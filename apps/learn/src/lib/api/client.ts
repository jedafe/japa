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
			if (typeof window !== 'undefined') {
				window.location.href = `https://japaconnect.com/login?redirectTo=${encodeURIComponent(window.location.href)}`;
			}
			return { data: null, error: { code: 'unauthorized', message: 'Session expired', status: 401 }, status: 401 };
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

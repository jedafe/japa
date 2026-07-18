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
	// Ref: internal/server/server.go — Control Panel routes are mounted under
	// /api/v1/control/* (inside the /api/v1 chi group). The .env contract states
	// client fetches use /api/v1/... (see PUBLIC_JC_API_URL). Several pages call
	// these endpoints as /control/... (no /api/v1 prefix), so normalize here to
	// keep a single routing contract and avoid 404s against the proxied backend.
	const normalizedPath =
		path.startsWith('/control') && !path.startsWith('/api/v1/control')
			? `/api/v1${path}`
			: path;
	const url = `${PUBLIC_JC_API_URL}${normalizedPath}`;

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

		if (res.status === 401) {
			return { data: null, error: { code: 'unauthorized', message: 'Session expired', status: 401 }, status: 401 };
		}

		if (res.status === 403) {
			return { data: null, error: { code: 'forbidden', message: 'Insufficient privileges', status: 403 }, status: 403 };
		}

		const ct = res.headers.get('content-type') ?? '';
		if (!ct.includes('application/json')) {
			const text = await res.text();
			return { data: null, error: { code: 'non_json_response', message: text.slice(0, 200) || res.statusText, status: res.status }, status: res.status };
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

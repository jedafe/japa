import { PUBLIC_JC_API_URL } from '$env/static/public';
import { csrfHeaders, refreshCsrfToken } from './csrf';
import { getAccessToken, setAccessToken, clearAccessToken } from './token';

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

// isCsrfRejection reports whether an error envelope denotes a CSRF token failure
// so the client can refresh the token and retry exactly once.
function isCsrfRejection(status: number, err: ApiErr['error'] | null): boolean {
	if (status !== 403) return false;
	return err?.code === 'csrf_token_invalid' || err?.code === 'csrf_token_missing';
}

async function attemptRefresh(): Promise<boolean> {
	if (refreshPromise) return refreshPromise;
	refreshPromise = (async () => {
		try {
			const res = await fetch(`${PUBLIC_JC_API_URL}/api/v1/auth/refresh`, {
				method: 'POST',
				credentials: 'include',
				headers: { 'Content-Type': 'application/json' },
			});
			if (res.ok) {
				const body = await res.json();
				const newToken = body?.data?.access_token;
				if (newToken) {
					setAccessToken(newToken);
					return true;
				}
			}
			return false;
		} catch {
			return false;
		} finally {
			refreshPromise = null;
		}
	})();
	return refreshPromise;
}

// dispatch performs a single fetch attempt with CSRF headers attached for unsafe
// methods, parsing the standard {data|error} envelope.
async function dispatch<T>(
	url: string,
	options: RequestInit,
	fetcher: typeof fetch,
	withCsrf: boolean
): Promise<ApiResult<T>> {
	const baseHeaders = options.headers as Record<string, string> | undefined;
	const headers = withCsrf ? csrfHeaders(options.method, baseHeaders) : baseHeaders;

	const accessToken = getAccessToken();
	const authHeader: Record<string, string> = accessToken
		? { Authorization: `Bearer ${accessToken}` }
		: {};

	const isFormData = typeof FormData !== 'undefined' && options.body instanceof FormData;
	const jsonHeader: Record<string, string> = isFormData ? {} : { 'Content-Type': 'application/json' };

	const res = await fetcher(url, {
		...options,
		credentials: 'include',
		headers: {
			...jsonHeader,
			...authHeader,
			...(headers ?? {}),
		},
	});

	if (res.status === 401) {
		const refreshed = await attemptRefresh();
		if (refreshed) {
			const retryToken = getAccessToken();
			const retryAuth: Record<string, string> = retryToken
				? { Authorization: `Bearer ${retryToken}` }
				: {};
			const retryRes = await fetcher(url, {
				...options,
				credentials: 'include',
				headers: {
					...jsonHeader,
					...retryAuth,
					...csrfHeaders(options.method, headers),
				},
			});
			if (retryRes.ok) {
				const body = await retryRes.json();
				return { data: (body as ApiOk<T>).data, error: null, status: retryRes.status };
			}
		}
		clearAccessToken();
		return { data: null, error: { code: 'unauthorized', message: 'Session expired', status: 401 }, status: 401 };
	}

	if (res.status === 204 || res.status === 205 || res.headers.get('content-length') === '0') {
		return { data: null, error: null, status: res.status };
	}

	const text = await res.text();
	if (!text) {
		return { data: null, error: null, status: res.status };
	}

	let body: any;
	try {
		body = JSON.parse(text);
	} catch {
		return { data: null, error: { code: 'invalid_response', message: 'Invalid JSON response', status: res.status }, status: res.status };
	}

	if (!res.ok || 'error' in body) {
		const err = 'error' in body ? body.error : { code: 'unknown', message: res.statusText, status: res.status };
		return { data: null, error: err, status: res.status };
	}

	return { data: (body as ApiOk<T>).data, error: null, status: res.status };
}

// retryAfterCsrfRefresh re-issues a fresh CSRF token and retries the request
// once. Returns the retry result (which may itself carry a 403 if the token
// still fails, at which point we surface the error).
async function retryAfterCsrfRefresh<T>(
	url: string,
	options: RequestInit,
	fetcher: typeof fetch
): Promise<ApiResult<T>> {
	await refreshCsrfToken(fetcher);
	return dispatch<T>(url, options, fetcher, true);
}

export async function apiFetch<T>(
	path: string,
	options: RequestInit = {},
	fetchFn?: typeof fetch
): Promise<ApiResult<T>> {
	const fetcher = fetchFn ?? fetch;
	const url = `${PUBLIC_JC_API_URL}/api/v1${path}`;
	const skipRefresh = options.headers && (options.headers as Record<string, string>)['X-Skip-Refresh'] === '1';
	const cleanOptions = { ...options };
	if (skipRefresh && cleanOptions.headers) {
		const h = { ...(cleanOptions.headers as Record<string, string>) };
		delete h['X-Skip-Refresh'];
		cleanOptions.headers = h;
	}

	try {
		const result = await dispatch<T>(url, cleanOptions, fetcher, true);

		if (!skipRefresh && isCsrfRejection(result.status, result.error)) {
			return retryAfterCsrfRefresh<T>(url, cleanOptions, fetcher);
		}

		return result;
	} catch (err) {
		return {
			data: null,
			error: { code: 'network_error', message: err instanceof Error ? err.message : 'Network error', status: 0 },
			status: 0,
		};
	}
}

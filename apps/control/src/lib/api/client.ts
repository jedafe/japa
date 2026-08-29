import { PUBLIC_JC_API_URL } from '$env/static/public';
import { buildApiUrl } from '@japaconnect/api-url';

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

const CSRF_HEADER = 'X-CSRF-Token';
const CSRF_COOKIE = 'csrf_token';
const MUTATING_METHODS = new Set(['POST', 'PUT', 'PATCH', 'DELETE']);

// readCsrfCookie returns the csrf_token cookie value, or null when absent.
// The cookie is intentionally non-HttpOnly (see internal/csrf) so client-side
// JS can read it for the double-submit pattern.
function readCsrfCookie(): string | null {
	if (typeof document === 'undefined') return null;
	const match = document.cookie
		.split(';')
		.map((c) => c.trim())
		.find((c) => c.startsWith(`${CSRF_COOKIE}=`));
	return match ? decodeURIComponent(match.slice(CSRF_COOKIE.length + 1)) : null;
}

// refreshCsrfCookie fetches a fresh double-submit CSRF token from the backend's
// /api/v1/csrf endpoint. The backend sets a new csrf_token cookie via Set-Cookie
// and echoes the token in the JSON body. After this call, readCsrfCookie() returns
// the new value. Returns true on success, false on failure.
let csrfRefreshPromise: Promise<boolean> | null = null;

async function refreshCsrfCookie(): Promise<boolean> {
	if (csrfRefreshPromise) return csrfRefreshPromise;
	csrfRefreshPromise = (async () => {
		try {
			const res = await fetch(buildApiUrl('/csrf', PUBLIC_JC_API_URL), {
				method: 'GET',
				credentials: 'include',
				headers: { 'Content-Type': 'application/json' },
			});
			return res.ok;
		} catch {
			return false;
		} finally {
			csrfRefreshPromise = null;
		}
	})();
	return csrfRefreshPromise;
}

// isCsrfRejection reports whether a 403 response is a CSRF token failure that
// warrants a token refresh + retry.
function isCsrfRejection(status: number, body: ApiOk<unknown> | ApiErr | null): boolean {
	if (status !== 403) return false;
	return body !== null && 'error' in body && body.error?.code === 'csrf_token_invalid';
}

// tryTokenRefresh attempts to refresh the access token using the httpOnly
// jc_refresh cookie. On success, updates the in-memory token via setToken
// and returns true. On failure, returns false. Uses a dynamic import to
// avoid a circular dependency with $lib/api (which imports this module).
// The X-Skip-Refresh header prevents the refresh call itself from
// triggering another refresh on 401.
let refreshPromise: Promise<boolean> | null = null;

async function tryTokenRefresh(): Promise<boolean> {
	if (refreshPromise) return refreshPromise;
	refreshPromise = (async () => {
		try {
			const res = await fetch(buildApiUrl('/auth/refresh', PUBLIC_JC_API_URL), {
				method: 'POST',
				credentials: 'include',
				headers: { 'Content-Type': 'application/json', 'X-Skip-Refresh': '1' },
			});
			if (!res.ok) return false;
			const body = await res.json().catch(() => null);
			if (!body?.data?.access_token) return false;
			const { setToken } = await import('$lib/api');
			setToken(body.data.access_token);
			return true;
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
	// Ref: internal/server/server.go — Control Panel routes are mounted at the
	// top-level /control/* group (NOT under /api/v1). The Vite dev proxy maps
	// /control -> http://localhost:8080, so pages call these endpoints as
	// /control/... and the request reaches the backend unchanged. Pass version: ''
	// so buildApiUrl does not prepend /api/v1 to these paths.
	const url = buildApiUrl(path, PUBLIC_JC_API_URL, { version: '' });
	const method = (options.method ?? 'GET').toString().toUpperCase();
	const isMutating = MUTATING_METHODS.has(method);

	async function doFetch(opts: RequestInit): Promise<{ res: Response; body: ApiOk<unknown> | ApiErr | null }> {
		const headers: Record<string, string> = {
			...(opts.headers as Record<string, string> | undefined),
		};
		if (opts.body) {
			headers['Content-Type'] = 'application/json';
		}

		const res = await fetcher(url, {
			...opts,
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
				return { res, body: { error: { code: 'non_json_response', message: text.slice(0, 200) || res.statusText, status: res.status } } as ApiErr };
			}
		}
		return { res, body };
	}

	try {
		let { res, body } = await doFetch(options);

		// On a CSRF rejection for a mutating request, refresh the token cookie
		// and retry once with the updated X-CSRF-Token header. This mirrors the
		// retry logic in the admin/main/learn/tools apps and prevents the user
		// from having to manually reload the page when the session-scoped
		// csrf_token cookie becomes stale.
		if (isMutating && isCsrfRejection(res.status, body)) {
			const refreshed = await refreshCsrfCookie();
			if (refreshed) {
				const newToken = readCsrfCookie();
				if (newToken) {
					const retryHeaders = {
						...(options.headers as Record<string, string> | undefined),
						[CSRF_HEADER]: newToken,
					};
					const retryOpts = { ...options, headers: retryHeaders };
					const retry = await doFetch(retryOpts);
					res = retry.res;
					body = retry.body;
				}
			}
		}

		// On a 401, attempt a token refresh using the httpOnly jc_refresh
		// cookie before surfacing the error. If the refresh succeeds, update
		// the in-memory token and retry the original request with the new
		// Authorization header. This mirrors the admin app's 401 handler and
		// prevents spurious "Session expired" errors when the 15-min access
		// token expires but the refresh cookie is still valid.
		// Ref: sections/02-authentication-sessions.md (Token model)
		// Guard: don't recurse if this request IS the refresh call (X-Skip-Refresh).
		if (
			res.status === 401 &&
			!(options.headers as Record<string, string>)?.['X-Skip-Refresh']
		) {
			const refreshOk = await tryTokenRefresh();
			if (refreshOk) {
				// Retry the original request with the updated Authorization header.
				const { getToken } = await import('$lib/api');
				const newToken = getToken();
				if (newToken) {
					const retryHeaders = {
						...(options.headers as Record<string, string> | undefined),
						Authorization: `Bearer ${newToken}`,
					};
					const retryOpts = { ...options, headers: retryHeaders };
					const retry = await doFetch(retryOpts);
					res = retry.res;
					body = retry.body;
				}
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

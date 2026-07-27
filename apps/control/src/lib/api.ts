import { apiFetch } from './api/client';

type FetchOptions = {
	method?: string;
	body?: unknown;
	signal?: AbortSignal;
};

type ApiResult<T> = { ok: true; data: T } | { ok: false; error: string };

let jwtToken: string | null = null;

export function setToken(token: string | null) {
	jwtToken = token;
}

// getToken returns the current JWT token (or null) so callers that need to
// build their own fetch (e.g. multipart uploads that cannot use JSON.stringify)
// can still attach the Authorization header.
export function getToken(): string | null {
	return jwtToken;
}

// readCookie returns the value of a document cookie by name, or null if absent.
// Used for the double-submit CSRF token (the backend issues a non-HttpOnly
// `csrf_token` cookie that the SPA echoes back as the X-CSRF-Token header).
function readCookie(name: string): string | null {
	if (typeof document === 'undefined') return null;
	const match = document.cookie
		.split(';')
		.map((c) => c.trim())
		.find((c) => c.startsWith(`${name}=`));
	return match ? decodeURIComponent(match.slice(name.length + 1)) : null;
}

// isMutating reports whether an HTTP method changes server state and therefore
// must carry a CSRF token.
function isMutating(method: string): boolean {
	const m = method.toUpperCase();
	return m === 'POST' || m === 'PATCH' || m === 'PUT' || m === 'DELETE';
}

export async function api<T>(path: string, opts: FetchOptions = {}): Promise<ApiResult<T>> {
	const headers: Record<string, string> = {};
	if (jwtToken) {
		headers['Authorization'] = `Bearer ${jwtToken}`;
	}

	// Double-submit CSRF: attach the token cookie value as the X-CSRF-Token
	// header on mutating requests (backend enforces this on /control/* routes).
	const method = opts.method ?? 'GET';
	if (isMutating(method)) {
		const csrf = readCookie('csrf_token');
		if (csrf) {
			headers['X-CSRF-Token'] = csrf;
		}
	}

	const result = await apiFetch<T>(path, {
		method: opts.method ?? 'GET',
		headers,
		body: opts.body ? JSON.stringify(opts.body) : undefined,
		signal: opts.signal,
	});

	if (result.error) {
		return { ok: false, error: result.error.message };
	}
	return { ok: true, data: result.data as T };
}

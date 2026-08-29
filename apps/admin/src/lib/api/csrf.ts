import { PUBLIC_JC_API_URL } from '$env/static/public';
import { buildApiUrl } from '@japaconnect/api-url';

// CSRF_TOKEN_COOKIE is the double-submit cookie the backend issues (non-HttpOnly)
// so the SPA can read it and echo it back as the X-CSRF-Token header.
const CSRF_TOKEN_COOKIE = 'csrf_token';
const CSRF_HEADER = 'X-CSRF-Token';

// cachedToken holds the most recently issued CSRF token. It is populated either
// by an explicit fetchCsrfToken() call or lazily from the csrf_token cookie if a
// token is already present (e.g. carried over from a previous session).
let cachedToken: string | null = null;

let fetchPromise: Promise<string | null> | null = null;

// readCsrfCookie returns the csrf_token cookie value, or null when absent.
// The cookie is intentionally NOT HttpOnly (see internal/csrf), so client-side
// JS can read it for the double-submit pattern.
function readCsrfCookie(): string | null {
	if (typeof document === 'undefined') return null;
	const match = document.cookie
		.split(';')
		.map((c) => c.trim())
		.find((c) => c.startsWith(`${CSRF_TOKEN_COOKIE}=`));
	if (!match) return null;
	return decodeURIComponent(match.slice(CSRF_TOKEN_COOKIE.length + 1));
}

// getCsrfToken returns a cached token, falling back to the cookie, falling back
// to a fresh fetch from the backend. It never throws — a missing token simply
// yields null and the request proceeds (the backend then rejects it with 403,
// which triggers a single refresh via refreshCsrfToken).
export async function getCsrfToken(fetchFn?: typeof fetch): Promise<string | null> {
	if (cachedToken) return cachedToken;
	const fromCookie = readCsrfCookie();
	if (fromCookie) {
		cachedToken = fromCookie;
		return cachedToken;
	}
	return fetchCsrfToken(fetchFn);
}

// fetchCsrfToken performs GET /api/v1/csrf, caches the returned token, and
// returns it. Concurrent callers share a single in-flight request.
export async function fetchCsrfToken(fetchFn?: typeof fetch): Promise<string | null> {
	if (fetchPromise) return fetchPromise;
	fetchPromise = (async () => {
		try {
			const res = await (fetchFn ?? fetch)(buildApiUrl('/csrf', PUBLIC_JC_API_URL), {
				method: 'GET',
				credentials: 'include',
				headers: { 'Content-Type': 'application/json' },
			});
			if (!res.ok) return null;
			const body = await res.json();
			const token = body?.data?.csrfToken as string | undefined;
			if (!token) return null;
			cachedToken = token;
			return token;
		} catch {
			return null;
		} finally {
			fetchPromise = null;
		}
	})();
	return fetchPromise;
}

// refreshCsrfToken clears the cache and fetches a fresh token. Called after the
// backend returns a 403 CSRF rejection so the next mutating request retries with
// a valid token.
export async function refreshCsrfToken(fetchFn?: typeof fetch): Promise<string | null> {
	cachedToken = null;
	return fetchCsrfToken(fetchFn);
}

// isUnsafeMethod reports whether a method changes server state and therefore
// must carry the X-CSRF-Token header.
export function isUnsafeMethod(method: string | undefined): boolean {
	switch ((method ?? 'GET').toUpperCase()) {
		case 'POST':
		case 'PUT':
		case 'PATCH':
		case 'DELETE':
			return true;
		default:
			return false;
	}
}

// csrfHeaders returns the X-CSRF-Token header (when a token is available and the
// method is unsafe) merged over the provided headers. Safe methods are left
// untouched. The token is resolved from the cache, cookie, or a fresh fetch.
export async function csrfHeaders(
	method: string | undefined,
	base: Record<string, string> = {}
): Promise<Record<string, string>> {
	if (!isUnsafeMethod(method)) return base;
	const token = cachedToken ?? readCsrfCookie();
	if (!token) {
		// Cookie might not be readable (different port/domain via Vite proxy).
		// Fetch a fresh token synchronously from the cache or endpoint.
		const freshToken = await getCsrfToken();
		if (!freshToken) return base;
		return { ...base, [CSRF_HEADER]: freshToken };
	}
	return { ...base, [CSRF_HEADER]: token };
}

export { CSRF_HEADER };

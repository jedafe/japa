import { browser } from '$app/environment';
import { PUBLIC_JC_API_URL } from '$env/static/public';

/**
 * Double-submit CSRF token store (Ref: §5.2).
 *
 * Seeded from the server-rendered page data (see +layout.svelte → seedCsrfToken).
 * On a 403 csrf_token_invalid, refreshCsrfToken() re-fetches GET /csrf and
 * updates the store so the retry carries the fresh token.
 *
 * The backend also sets a csrf_token cookie (non-httpOnly) on the /csrf fetch;
 * the browser echoes it back automatically, and we attach the X-CSRF-Token
 * header from this store for the double-submit pattern.
 */

let csrfToken = $state<string | null>(null);

export function getCsrfToken(): string | null {
	return csrfToken;
}

export function seedCsrfToken(token: string | null): void {
	csrfToken = token;
}

export async function refreshCsrfToken(): Promise<boolean> {
	if (!browser) return false;
	try {
		const res = await fetch(`${PUBLIC_JC_API_URL}/csrf`, { credentials: 'include' });
		if (!res.ok) return false;
		const body = await res.json();
		const token = body?.data?.csrfToken ?? body?.csrfToken ?? null;
		if (token) {
			csrfToken = token;
			return true;
		}
		return false;
	} catch {
		return false;
	}
}

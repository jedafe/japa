// Double-submit CSRF for the Docs surface (§5.2). The token lives in the
// non-httpOnly csrf_token cookie — mirrored onto the browser by
// hooks.server.ts — so reads come from document.cookie and refreshes hit
// GET /csrf directly.
import { PUBLIC_JC_API_URL } from '$app/env/public';
import { buildApiUrl } from '@japaconnect/api-url';
import { getCookie } from '#lib/api/client.js';

export function getCsrfToken(): string | null {
	return getCookie('csrf_token') ?? null;
}

export async function refreshCsrfToken(): Promise<void> {
	const base = (PUBLIC_JC_API_URL ?? '/api/v1').replace(/\/+$/, '');
	await fetch(buildApiUrl('/csrf', base), { credentials: 'include' });
}

// Browser-side access-token store (memory only — never persisted).
//
// The HttpOnly jc_token cookie is the canonical session credential for SSR,
// but it is invisible to client-side fetch and — being host-scoped to the
// frontend origin — never reaches api.<domain>. The Go API authenticates
// Authorization: Bearer only, so production browser calls need the token in
// JS memory (same model as apps/admin and apps/control).
//
// Seeded from server-rendered page data (root layout) and refreshed through
// the SvelteKit /api/auth/refresh endpoint, which returns the new access
// token in its JSON body. Token lifetime is short (15 min); a stale token
// yields 401 → client.ts refreshes and retries.
let token: string | null = $state(null);

/** Seed/replace the token from SSR page data or a refresh response. */
export function seedAccessToken(value: string | null): void {
	if (value && token !== value) token = value;
}

/** Current access token, or null for guests / expired sessions. */
export function getAccessToken(): string | null {
	return token;
}

/** Drop the token (logout / confirmed session loss). */
export function clearAccessToken(): void {
	token = null;
}

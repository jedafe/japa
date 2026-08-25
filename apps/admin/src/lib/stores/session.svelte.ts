import { api } from '$lib/api';
import { refreshAccessToken } from '$lib/api/client';
import { setUser, setPermissions, clearUser } from '$lib/stores/auth.svelte';
import { getAccessToken } from '$lib/api/token';

/**
 * Restore the session by fetching /me and /me/permissions and updating the
 * auth stores. Returns true if the session is valid, false otherwise.
 *
 * Shared by the root layout (onMount), the login page (after setting the
 * access token), and the ImpersonationBanner (after ending impersonation) so
 * that none of them need a hard `window.location.href` reload to refresh auth
 * state — they can use `goto()` instead.
 *
 * If `tryRefresh` is true and no access token is present, a /auth/refresh
 * attempt is made first (root-layout behaviour).
 */
export async function restoreSession(opts: { tryRefresh?: boolean } = {}): Promise<boolean> {
	const { tryRefresh = false } = opts;

	if (tryRefresh && !getAccessToken()) {
		// Use the shared refresh promise so this does not race with 401 retries
		// from concurrent initial requests and consume the one-time cookie twice.
		const ok = await refreshAccessToken();
		if (!ok) {
			clearUser();
			return false;
		}
	}

	try {
		const userResult = await api<{ user: any }>('/me');
		if (userResult.ok) {
			setUser(userResult.data.user);

			const permResult = await api<{ permissions: string[] }>('/me/permissions');
			if (permResult.ok) {
				setPermissions(permResult.data.permissions ?? []);
			} else {
				setPermissions([]);
			}
			return true;
		}
		clearUser();
		return false;
	} catch {
		clearUser();
		return false;
	}
}

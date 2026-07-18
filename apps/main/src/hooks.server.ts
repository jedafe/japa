import type { Handle } from '@sveltejs/kit';
import { building } from '$app/environment';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { SessionUser } from '$lib/types';

export const handle: Handle = async ({ event, resolve }) => {
	// Hostname excludes the port; the backend resolves the tenant from it.
	event.locals.tenantHost = event.url.hostname;
	event.locals.token = event.cookies.get('jc_token') ?? null;
	event.locals.theme = event.cookies.get('jc_theme') === 'dark' ? 'dark' : 'light';
	event.locals.user = null;

	// Resolve the session. The main app is public: an absent or invalid token
	// simply means "guest" — never a redirect to login (that's the admin app's job).
	if (event.locals.token && !building) {
		try {
			event.locals.user = await apiGet<SessionUser>(apiContext(event), '/me');
		} catch (err) {
			if (err instanceof ApiError && (err.status === 401 || err.status === 403)) {
				// Stale credential — drop it and continue as a guest.
				event.cookies.delete('jc_token', { path: '/' });
				event.locals.token = null;
			}
			// Any other failure (backend unreachable) also degrades to guest.
		}
	}

	return resolve(event, {
		transformPageChunk: ({ html }) => html.replace('%jc.theme%', event.locals.theme)
	});
};

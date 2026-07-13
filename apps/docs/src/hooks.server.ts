import type { Handle } from '@sveltejs/kit';
import { PUBLIC_JC_API_URL } from '$env/static/public';

export const handle: Handle = async ({ event, resolve }) => {
	const sessionCookie = event.cookies.get('jc_token');

	if (sessionCookie) {
		event.locals.token = sessionCookie;
		try {
			const meRes = await fetch(`${PUBLIC_JC_API_URL}/api/v1/me`, {
				headers: { Authorization: `Bearer ${sessionCookie}` },
			});

			if (meRes.ok) {
				const body = await meRes.json();
				event.locals.user = body.data;
			} else {
				event.locals.user = null;
			}
		} catch {
			event.locals.user = null;
		}
	} else {
		event.locals.user = null;
		event.locals.token = null;
	}

	event.locals.isAuth = !!event.locals.user;

	try {
		const flagsRes = await fetch(`${PUBLIC_JC_API_URL}/api/v1/feature-flags`, {
			headers: sessionCookie ? { Authorization: `Bearer ${sessionCookie}` } : {},
		});
		if (flagsRes.ok) {
			const { data } = await flagsRes.json();
			event.locals.featureFlags = data ?? {};
		} else {
			event.locals.featureFlags = {};
		}
	} catch {
		event.locals.featureFlags = {};
	}

	const response = await resolve(event);

	// Set cache headers based on path
	const path = event.url.pathname;

	if (path.startsWith('/_app/')) {
		// Static assets - cache aggressively (1 year, immutable)
		response.headers.set('Cache-Control', 'public, max-age=31536000, immutable');
	} else if (path.startsWith('/api/') || path.startsWith('/imgproxy/')) {
		// API and proxied responses - short cache with revalidation
		response.headers.set('Cache-Control', 'private, max-age=0, must-revalidate');
	} else if (path.match(/\.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2|ttf|eot)$/)) {
		// Static files - cache for 1 week
		response.headers.set('Cache-Control', 'public, max-age=604800, stale-while-revalidate=86400');
	} else {
		// HTML pages - no cache, must revalidate
		response.headers.set('Cache-Control', 'no-cache, must-revalidate');
		response.headers.set('Pragma', 'no-cache');
	}

	return response;
};

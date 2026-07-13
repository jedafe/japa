import { redirect, type Handle, type HandleServerError } from '@sveltejs/kit';
import { PUBLIC_JC_API_URL } from '$env/static/public';

const PROTECTED_ROUTES = ['/certificates', '/dashboard'];

function isProtectedRoute(pathname: string): boolean {
	return PROTECTED_ROUTES.some(
		(route) => pathname === route || pathname.startsWith(`${route}/`)
	);
}

export const handle: Handle = async ({ event, resolve }) => {
	const token = event.cookies.get('jc_token');

	if (token) {
		try {
			const meRes = await fetch(`${PUBLIC_JC_API_URL}/api/v1/me`, {
				headers: { Authorization: `Bearer ${token}` },
			});

			if (meRes.ok) {
				const { data } = await meRes.json();
				event.locals.user = data;
				event.locals.token = token;
			} else {
				event.locals.user = null;
				event.locals.token = null;
			}
		} catch {
			event.locals.user = null;
			event.locals.token = null;
		}
	} else {
		event.locals.user = null;
		event.locals.token = null;
	}

	try {
		const flagsRes = await fetch(`${PUBLIC_JC_API_URL}/api/v1/feature-flags`, {
			headers: token ? { Authorization: `Bearer ${token}` } : {},
		});
		if (flagsRes.ok) {
			const { data } = await flagsRes.json();
			event.locals.featureFlags = data;
			if (data && data['feature.learning_platform'] === false) {
				event.locals.platformDisabled = true;
			}
		}
	} catch {
		event.locals.featureFlags = {};
	}

	if (isProtectedRoute(event.url.pathname) && !event.locals.user) {
		const redirectTo = encodeURIComponent(event.url.pathname + event.url.search);
		throw redirect(303, `/login?redirectTo=${redirectTo}`);
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

export const handleError: HandleServerError = ({ error }) => {
	return {
		code: 'INTERNAL_ERROR',
		message: 'Internal error',
	};
};

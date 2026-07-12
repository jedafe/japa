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
	return response;
};

export const handleError: HandleServerError = ({ error }) => {
	return {
		code: 'INTERNAL_ERROR',
		message: 'Internal error',
	};
};

import { redirect, type Handle, type HandleServerError } from '@sveltejs/kit';
import { API_BASE_URL } from '$lib/server/api';

/** Routes that require an authenticated user. */
const PROTECTED_ROUTES = ['/certificates'];

/** Check if a pathname requires authentication. */
function isProtectedRoute(pathname: string): boolean {
	return PROTECTED_ROUTES.some(
		(route) => pathname === route || pathname.startsWith(`${route}/`)
	);
}

export const handle: Handle = async ({ event, resolve }) => {
	const token = event.cookies.get('learn_token') ?? event.cookies.get('jc_token');

	if (token) {
		try {
			const res = await fetch(`${API_BASE_URL}/users/me`, {
				headers: { Authorization: `Bearer ${token}` }
			});
			if (res.ok) {
				const { data } = await res.json();
				event.locals.user = {
					id: data.id,
					email: data.email,
					username: data.username,
					displayName: data.display_name,
					avatarUrl: data.avatar_url
				};
			} else {
				event.locals.user = null;
			}
		} catch {
			event.locals.user = null;
		}
	} else {
		event.locals.user = null;
	}

	// Redirect unauthenticated users away from protected routes.
	if (isProtectedRoute(event.url.pathname) && !event.locals.user) {
		throw redirect(303, '/login');
	}

	const response = await resolve(event);
	return response;
};

export const handleError: HandleServerError = ({ error }) => {
	return {
		code: 'INTERNAL_ERROR',
		message: 'Internal error'
	};
};

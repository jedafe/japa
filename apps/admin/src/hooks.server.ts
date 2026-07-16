import { redirect, type Handle, type HandleServerError } from '@sveltejs/kit';
import { building } from '$app/environment';
import * as Sentry from '@sentry/sveltekit';
import { env } from '$env/dynamic/private';

const API_BASE = env.PRIVATE_API_BASE_URL || 'http://localhost:8080';

const PUBLIC_ROUTES = ['/login', '/_app'];

function isPublicRoute(pathname: string): boolean {
	return PUBLIC_ROUTES.some((route) => pathname === route || pathname.startsWith(route + '/'));
}

export const handle: Handle = async ({ event, resolve }) => {
	if (building) {
		return resolve(event);
	}

	if (isPublicRoute(event.url.pathname) || event.url.pathname.includes('.')) {
		return resolve(event);
	}

	const token = event.cookies.get('jc_token');

	// Dev mode bypass: try to get a real admin token from the backend
	const isDev = env.NODE_ENV !== 'production';
	const devMode = isDev || env.PRIVATE_API_BASE_URL === 'http://localhost:8080';

	if (!token && devMode) {
		// Try to login with admin credentials to get a real token
		try {
			const loginRes = await fetch(`${API_BASE}/api/v1/auth/login`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					email: 'admin@japaconnect.com',
					password: 'Admin123!'
				})
			});

			if (loginRes.ok) {
				const { data } = await loginRes.json();
				const realToken = data?.access_token;

				if (realToken) {
					// Set the real token in the cookie
					event.cookies.set('jc_token', realToken, {
						path: '/',
						httpOnly: true,
						secure: false,
						sameSite: 'lax',
						maxAge: 3600 * 24 * 7
					});

					// Get user info with the real token
					const meRes = await fetch(`${API_BASE}/api/v1/me`, {
						headers: { Authorization: `Bearer ${realToken}` }
					});

					if (meRes.ok) {
						const { data: userData } = await meRes.json();
						event.locals.user = userData;
						event.locals.token = realToken;
					}
				}
			}
		} catch {
			// Fallback to mock user if backend is unavailable
			event.locals.user = {
				id: 'dev-admin-001',
				username: 'dev_admin',
				email: 'dev@japaconnect.com',
				displayName: 'Dev Admin',
				groups: ['super_admin'],
				avatar_url: null,
			};
			event.locals.token = 'dev-mock-token';
		}
	} else if (!token) {
		throw redirect(303, '/login');
	} else {
		try {
			const meRes = await fetch(`${API_BASE}/api/v1/me`, {
				headers: { Authorization: `Bearer ${token}` },
			});

			if (meRes.ok) {
				const { data } = await meRes.json();
				event.locals.user = data;
				event.locals.token = token;
			} else {
				event.cookies.delete('jc_token', { path: '/' });
				throw redirect(303, '/login');
			}
		} catch (e) {
			if (e instanceof Response) throw e;
			event.cookies.delete('jc_token', { path: '/' });
			throw redirect(303, '/login');
		}
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
	Sentry.captureException(error);
	return {
		code: 'INTERNAL_ERROR',
		message: 'Internal error',
	};
};

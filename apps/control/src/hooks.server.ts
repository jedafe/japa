import { redirect, type Handle, type HandleServerError } from '@sveltejs/kit';
import * as Sentry from '@sentry/sveltekit';
import { env } from '$env/dynamic/private';

const API_BASE = env.PRIVATE_API_BASE_URL || 'http://localhost:8080';

const PUBLIC_ROUTES = ['/login', '/_app'];

function isPublicRoute(pathname: string): boolean {
	return PUBLIC_ROUTES.some((route) => pathname === route || pathname.startsWith(route + '/'));
}

export const handle: Handle = async ({ event, resolve }) => {
	if (isPublicRoute(event.url.pathname) || event.url.pathname.includes('.')) {
		return resolve(event);
	}

	const token = event.cookies.get('jc_token');

	if (!token) {
		throw redirect(303, '/login');
	}

	try {
		const meRes = await fetch(`${API_BASE}/api/v1/me`, {
			headers: { Authorization: `Bearer ${token}` },
		});

		if (meRes.ok) {
			const { data } = await meRes.json();
			event.locals.user = data;
			event.locals.token = token;

			// Super_admin gets wildcard permission so the layout's
			// hasPermission() check never blocks a page.
			if (data.user_group_name === 'super_admin') {
				data.permissions = ['*'];
			}

			if (data.user_group_name !== 'super_admin') {
				return new Response(
					`<!DOCTYPE html><html><head><title>403</title></head><body>
					<h1>403 — Insufficient Privileges</h1>
					<p>The Control Panel is restricted to super_admin users only.</p>
					<a href="/login">Try a different account</a>
					</body></html>`,
					{ status: 403, headers: { 'Content-Type': 'text/html' } }
				);
			}
		} else {
			event.cookies.delete('jc_token', { path: '/' });
			throw redirect(303, '/login');
		}
	} catch (e) {
		if (e instanceof Response) throw e;
		event.cookies.delete('jc_token', { path: '/' });
		throw redirect(303, '/login');
	}

	const response = await resolve(event);
	return response;
};

export const handleError: HandleServerError = ({ error }) => {
	Sentry.captureException(error);
	return {
		code: 'INTERNAL_ERROR',
		message: 'Internal error',
	};
};

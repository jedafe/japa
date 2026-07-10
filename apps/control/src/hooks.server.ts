import { redirect, type Handle, type HandleServerError } from '@sveltejs/kit';
import { PUBLIC_JC_API_URL } from '$env/static/public';

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
		const meRes = await fetch(`${PUBLIC_JC_API_URL}/api/v1/me`, {
			headers: { Authorization: `Bearer ${token}` },
		});

		if (meRes.ok) {
			const { data } = await meRes.json();
			event.locals.user = data;
			event.locals.token = token;

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
	return {
		code: 'INTERNAL_ERROR',
		message: 'Internal error',
	};
};

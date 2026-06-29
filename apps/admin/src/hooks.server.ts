import type { Handle, HandleServerError } from '@sveltejs/kit';
import { API_BASE } from '$lib/server/api';

export const handle: Handle = async ({ event, resolve }) => {
	const path = event.url.pathname;

	// Allow public paths and static assets through without auth
	if (path.startsWith('/login') || path.startsWith('/_app') || path.includes('.')) {
		return resolve(event);
	}

	const token = event.cookies.get('admin_token');

	if (!token) {
		// No token — redirect to login
		return new Response(null, {
			status: 303,
			headers: { location: '/login' }
		});
	}

	// Validate token against backend
	try {
		const res = await fetch(`${API_BASE}/users/me`, {
			headers: { Authorization: `Bearer ${token}` }
		});

		if (res.ok) {
			const body = await res.json();
			const user = body.data ?? body;
			event.locals.user = {
				id: user.id,
				email: user.email,
				username: user.username,
				displayName: user.display_name ?? user.username
			};
			event.locals.token = token;
		} else {
			// Invalid token — clear cookie and redirect to login
			event.cookies.delete('admin_token', { path: '/' });
			return new Response(null, {
				status: 303,
				headers: { location: '/login' }
			});
		}
	} catch {
		// Backend unreachable — clear cookie and redirect to login
		event.cookies.delete('admin_token', { path: '/' });
		return new Response(null, {
			status: 303,
			headers: { location: '/login' }
		});
	}

	return resolve(event);
};

export const handleError: HandleServerError = ({ error }) => {
	return {
		code: 'INTERNAL_ERROR',
		message: 'Internal error'
	};
};

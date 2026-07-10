import type { Handle } from '@sveltejs/kit';
import { PUBLIC_JC_API_URL } from '$env/static/public';

export const handle: Handle = async ({ event, resolve }) => {
	const sessionCookie = event.cookies.get('jc_token');

	if (sessionCookie) {
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
	}

	event.locals.isAuth = !!event.locals.user;

	const response = await resolve(event);
	return response;
};

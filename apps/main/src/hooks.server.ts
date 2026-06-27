import type { Handle } from '@sveltejs/kit';
import { API_BASE_URL } from '$lib/server/api';

export const handle: Handle = async ({ event, resolve }) => {
	const token = event.cookies.get('jc_token');

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
					avatarUrl: data.avatar_url,
					profileType: data.profile_type,
					status: data.status
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

	event.locals.featureFlags = {};
	event.locals.notificationCount = 0;

	const response = await resolve(event);
	return response;
};

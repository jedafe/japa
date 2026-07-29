// Ref: japaconnect-engineering/sections/02-authentication-sessions.md
// Gate the signup form on the general.registration_open system setting.
// When registration is closed, show a "registration closed" message instead
// of the signup form. Authenticated users are redirected by the (auth) layout.
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	let registrationOpen = true;
	try {
		const settings = (await apiGet<Record<string, unknown>>(ctx, '/site/settings/public')) ?? {};
		registrationOpen = settings['general.registration_open'] !== false;
	} catch {
		// If settings can't be fetched, default to open (fail open, not closed).
		registrationOpen = true;
	}

	if (!registrationOpen) {
		throw error(403, { code: 'registration_closed', message: 'Registration is currently closed. Please check back later.' });
	}

	return { registrationOpen };
};

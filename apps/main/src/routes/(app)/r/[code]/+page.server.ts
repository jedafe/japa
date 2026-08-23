import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { API_V1 } from '$lib/server/api';

/**
 * Affiliate short-link redirect at /r/[code].
 * The backend's GET /api/v1/r/:code returns a 302 to the configured
 * destination, so we call it with redirect: 'manual' and forward the
 * Location header as a SvelteKit redirect.
 *
 * Ref: japaconnect-engineering/sections/56-user-affiliate-program.md
 */
export const load: PageServerLoad = async (event) => {
	const code = event.params.code;

	const res = await event.fetch(`${API_V1}/r/${encodeURIComponent(code)}`, {
		method: 'GET',
		redirect: 'manual'
	});

	const location = res.headers.get('location');
	if (res.status >= 300 && res.status < 400 && location) {
		throw redirect(302, location);
	}

	throw error(404, { code: 'not_found', message: 'Short link not found' });
};

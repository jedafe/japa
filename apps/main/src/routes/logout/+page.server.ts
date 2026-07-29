// Ref: japaconnect-engineering/sections/02-authentication-sessions.md
// POST/GET /logout — server-side logout. Calls the backend to revoke the
// refresh token + access-token JTI, clears the jc_token cookie, and redirects
// to /login. Triggered by the UserDropdown "Log out" link.
import { redirect } from '@sveltejs/kit';
import type { Actions, PageServerLoad } from './$types';
import { apiContext, backendLogout, clearSessionCookie } from '$lib/server/auth';

// A plain navigation to /logout (GET) logs the user out too — the dropdown
// uses an <a href="/logout">, so we handle both GET and POST here.
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	await backendLogout(ctx);
	clearSessionCookie(event);
	throw redirect(303, '/login');
};

export const actions: Actions = {
	default: async (event) => {
		const ctx = apiContext(event);
		await backendLogout(ctx);
		clearSessionCookie(event);
		throw redirect(303, '/login');
	}
};

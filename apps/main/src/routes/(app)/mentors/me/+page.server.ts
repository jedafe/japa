// Ref: japaconnect-engineering/sections/102-mentorship-matching.md
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { MentorProfile, MentorshipConnection } from '$lib/types';

/**
 * My Mentorship at /mentors/me — auth-gated. Loads the caller's own mentor
 * profile (GET /me/mentor-profile) plus outgoing (GET /me/mentorship/outgoing)
 * and incoming (GET /me/mentorship/incoming) connections. Each call is wrapped
 * in try/catch so a single failing endpoint never blanks the page.
 * Feature-flagged via `mentorship.enabled` (§102): 404 when disabled.
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname)}`);
	}

	const { features } = await event.parent();
	if (!features['mentorship.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Mentorship is not available' });
	}

	const ctx = apiContext(event);

	let profile: MentorProfile | null = null;
	let outgoing: MentorshipConnection[] = [];
	let incoming: MentorshipConnection[] = [];
	let loadError = false;

	try {
		profile = await apiGet<MentorProfile>(ctx, '/me/mentor-profile');
	} catch {
		profile = null;
	}

	try {
		outgoing = (await apiList<MentorshipConnection>(ctx, '/me/mentorship/outgoing')).items;
	} catch {
		outgoing = [];
	}

	try {
		incoming = (await apiList<MentorshipConnection>(ctx, '/me/mentorship/incoming')).items;
	} catch {
		incoming = [];
	}

	if (!profile && outgoing.length === 0 && incoming.length === 0) {
		loadError = true;
	}

	return { profile, outgoing, incoming, loadError };
};

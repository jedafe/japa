// Ref: japaconnect-engineering/sections/102-mentorship-matching.md
// (2026-11-28 amendment — member mentor applications)
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '#lib/server/api.js';
import type { MentorProfile, MyMentorApplicationStatus } from '#lib/types/index.js';

/**
 * Become-a-mentor wizard at /become-mentor — auth + mentorship.enabled
 * gated like every mentorship surface.
 *
 * GET /me/mentor-application is the single eligibility probe for all
 * become-mentor surfaces: `eligible` (caller already holds
 * mentorship.become_mentor — quick-enable form), `is_mentor` (done
 * state), and `application` (pending/declined states). Callers holding
 * neither apply nor become_mentor get a backend 403 which degrades to a
 * "not available for your account" notice rather than a page error.
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['mentorship.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Mentorship is not available' });
	}

	if (!event.locals.user) {
		throw redirect(
			307,
			`/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`
		);
	}

	const ctx = apiContext(event);
	const status = await apiGet<MyMentorApplicationStatus>(ctx, '/me/mentor-application').catch(
		() => null
	);

	// Eligible members get the mentor profile too so the quick-enable form
	// seeds capacity/areas/bio from any existing draft.
	const profile =
		status?.eligible === true
			? await apiGet<MentorProfile>(ctx, '/me/mentor-profile').catch(() => null)
			: null;

	return { status, profile };
};

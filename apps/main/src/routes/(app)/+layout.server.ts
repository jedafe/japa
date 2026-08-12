import type { LayoutServerLoad } from './$types';
import { redirect } from '@sveltejs/kit';
import { apiContext, apiGet } from '$lib/server/api';
import { normalizeConfig, type CardStyleConfig } from '$lib/feed/cardStyle';
import type { OnboardingStatus } from '$lib/types';

/**
 * Shell-scoped data for every `(app)` route: the per-type card-style config so
 * the Home feed can resolve a card style per content type. The endpoint is
 * public, but the fetch degrades to the reddit default if it is unavailable —
 * a missing setting must never blank the shell.
 *
 * ADR-060: the Japa University branded host (university.japaconnect.com and
 * configured custom domains) is served by the dedicated Learn app, not the
 * main app. Redirect any request to that host to learn.japaconnect.com so the
 * catalog, tracks, lesson player, instructor, and earnings surfaces live in
 * exactly one place. Ref: audit finding C2.
 *
 * Section 09 Product Tour: for authed users, fetch `tour_dismissed` from the
 * onboarding status so the layout can guard the `?tour=1` auto-trigger — a
 * dismissed tour must never re-trigger automatically. The explicit "Replay
 * tour" link uses `?tour=replay` to override the guard.
 */
export const load: LayoutServerLoad = async (event) => {
	if (event.locals.isUniversityHost) {
		throw redirect(302, 'https://learn.japaconnect.com');
	}

	const ctx = apiContext(event);

	let cardStyleConfig: CardStyleConfig;
	try {
		cardStyleConfig = normalizeConfig(await apiGet<unknown>(ctx, '/feed/card-style'));
	} catch {
		cardStyleConfig = { default: 'reddit', byType: {} };
	}

	// Section 09: fetch tour_dismissed for the ?tour=1 auto-trigger guard.
	let tourDismissed = false;
	if (event.locals.user) {
		try {
			const status = await apiGet<OnboardingStatus>(ctx, '/onboarding/status');
			tourDismissed = !!status?.tour_dismissed;
		} catch {
			tourDismissed = false;
		}
	}

	return { cardStyleConfig, tourDismissed };
};

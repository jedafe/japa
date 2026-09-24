// Ref: japaconnect-engineering/sections/88-ask-an-expat.md
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { AMAThread, GeoCountry } from '$lib/types';

/**
 * Edit-AMA form at /ama/[id]/edit — auth-gated, host-or-moderate.
 * Loads the AMA thread by id and the country list for display labels.
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['forum.ama.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Ask an Expat AMA is not available' });
	}

	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const ctx = apiContext(event);
	const id = event.params.id;

	let session: AMAThread;
	try {
		// ?raw=1 keeps authored {{...}} literals unresolved for the edit form.
		session = await apiGet<AMAThread>(ctx, `/forum/ama/${id}?raw=1`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'AMA session not found' });
		}
		throw error(500, { code: 'load_failed', message: 'Failed to load AMA' });
	}

	const canEdit =
		session.author_user_id === event.locals.user.id ||
		event.locals.user.permissions?.includes('forum.ama.moderate');
	if (!canEdit) {
		throw error(403, { code: 'not_owner', message: 'Only the host or a moderator can edit this AMA' });
	}

	let countries: GeoCountry[] = [];
	try {
		const { items } = await apiList<GeoCountry>(ctx, '/geo/countries');
		countries = items;
	} catch {
		countries = [];
	}

	return { session, countries, user: event.locals.user, features };
};

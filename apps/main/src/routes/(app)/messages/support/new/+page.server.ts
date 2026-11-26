// Ref: japaconnect-engineering/sections/38-tickets.md (user portal)
// Loads the public ticket types so the user can pick one when filing a new
// ticket. Auth-gated; the backend filters to is_public=true types for users.
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '#lib/server/api.js';
import type { TicketType } from '#lib/types/index.js';

export const load: PageServerLoad = async (event) => {
	// Same gate as the workbench — the standalone form must not outlive
	// the feature flag.
	const { features } = await event.parent();
	if (features && features['feature.tickets'] === false) {
		throw error(404, { code: 'feature_disabled', message: 'Support tickets are not available' });
	}

	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const ctx = apiContext(event);
	const types = (await apiList<TicketType>(ctx, '/ticket-types?is_public=true')).items;
	return { types };
};

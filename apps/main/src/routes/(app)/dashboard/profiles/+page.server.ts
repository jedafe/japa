// Ref: japaconnect-engineering/sections/33-professional-profiles.md (my profiles)
// Lists the professional profiles owned by the current user. Auth-gated;
// the backend returns profiles scoped to the caller's owner_user_id.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { ProfessionalProfile } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}
	const ctx = apiContext(event);
	const profiles = (await apiList<ProfessionalProfile>(ctx, '/profiles?page_size=100')).items;
	return { profiles };
};

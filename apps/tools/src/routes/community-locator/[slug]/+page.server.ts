import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGetOrNull } from '$lib/server/api';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { CommunityOrgWithAi } from '$lib/types';

/**
 * Community organisation detail page — §101, ADR-109.
 *
 * API: GET /community-locator/{slug}
 *
 * Ref: §101 (Diaspora Community Locator), ROADMAP-TOOLS 02.06.
 */
export const load: PageServerLoad = async (event) => {
	// Gate: if the tool is disabled, surface a 404 so the route is unreachable.
	const tool = getTool('community-locator')!;
	if (!isToolEnabled(tool, event.locals.features)) {
		throw error(404, { code: 'tool_disabled', message: 'Community Locator is not available' });
	}

	const ctx = apiContext(event);
	const org = await apiGetOrNull<CommunityOrgWithAi>(ctx, `/community-locator/${event.params.slug}`);

	if (!org) {
		throw error(404, { code: 'community_org_not_found', message: 'Community organisation not found' });
	}

	return { org, user: event.locals.user };
};

import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { CommunityOrgWithAi } from '$lib/types';

/**
 * Community Locator organisation detail page — Section 101, ADR-109.
 *
 * API: GET /community-locator/:slug
 *
 * Ref: japaconnect-engineering/sections/101-diaspora-community-locator.md
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['community_locator.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Community Locator is not available' });
	}

	const ctx = apiContext(event);
	const slug = event.params.slug;

	let org: CommunityOrgWithAi | null = null;
	try {
		org = await apiGet<CommunityOrgWithAi>(ctx, `/community-locator/${slug}`);
	} catch {
		org = null;
	}

	if (!org) {
		throw error(404, { code: 'org_not_found', message: 'Organisation not found' });
	}

	return { org };
};

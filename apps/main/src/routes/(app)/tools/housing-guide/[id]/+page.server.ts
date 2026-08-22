import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { HousingGuideWithAi } from '$lib/types';

/**
 * Housing Guide detail page — Section 96, ADR-104.
 *
 * API: GET /housing-guide/:id
 *
 * Ref: japaconnect-engineering/sections/96-housing-guide.md
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['housing_guide.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Housing Guide is not available' });
	}

	const ctx = apiContext(event);
	const guideId = event.params.id;

	let guide: HousingGuideWithAi | null = null;
	try {
		guide = await apiGet<HousingGuideWithAi>(ctx, `/housing-guide/${guideId}`);
	} catch {
		guide = null;
	}

	if (!guide) {
		throw error(404, { code: 'housing_guide_not_found', message: 'Housing guide not found' });
	}

	return { guide };
};

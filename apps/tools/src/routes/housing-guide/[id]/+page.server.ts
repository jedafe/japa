import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGetOrNull } from '$lib/server/api';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { HousingGuideWithAi } from '$lib/types';

/**
 * Housing guide detail page — §96, ADR-104.
 *
 * API: GET /housing-guide/{id}
 *
 * Ref: §96 (Housing Guide), ROADMAP-TOOLS 02.06.
 */
export const load: PageServerLoad = async (event) => {
	// Gate: if the tool is disabled, surface a 404 so the route is unreachable.
	const tool = getTool('housing-guide')!;
	if (!isToolEnabled(tool, event.locals.features)) {
		throw error(404, { code: 'tool_disabled', message: 'Housing Guide is not available' });
	}

	const ctx = apiContext(event);
	const guide = await apiGetOrNull<HousingGuideWithAi>(ctx, `/housing-guide/${event.params.id}`);

	if (!guide) {
		throw error(404, { code: 'housing_guide_not_found', message: 'Housing guide not found' });
	}

	return { guide, user: event.locals.user };
};

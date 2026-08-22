import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGetOrNull } from '$lib/server/api';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { AttestationPathwayWithAi } from '$lib/types';

/**
 * Attestation pathway detail page — §97, ADR-105.
 *
 * API: GET /attestation/{id}
 *
 * Ref: §97 (Attestation Guide), ROADMAP-TOOLS 02.06.
 */
export const load: PageServerLoad = async (event) => {
	// Gate: if the tool is disabled, surface a 404 so the route is unreachable.
	const tool = getTool('attestation-guide')!;
	if (!isToolEnabled(tool, event.locals.features)) {
		throw error(404, { code: 'tool_disabled', message: 'Attestation Guide is not available' });
	}

	const ctx = apiContext(event);
	const pathway = await apiGetOrNull<AttestationPathwayWithAi>(ctx, `/attestation/${event.params.id}`);

	if (!pathway) {
		throw error(404, { code: 'attestation_pathway_not_found', message: 'Attestation pathway not found' });
	}

	return { pathway, user: event.locals.user };
};

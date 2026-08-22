import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { AttestationPathwayWithAi } from '$lib/types';

/**
 * Attestation Guide detail page — Section 97, ADR-105.
 *
 * API: GET /attestation/:id
 *
 * Ref: japaconnect-engineering/sections/97-attestation-guide.md
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['attestation_guide.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Attestation Guide is not available' });
	}

	const ctx = apiContext(event);
	const pathwayId = event.params.id;

	let pathway: AttestationPathwayWithAi | null = null;
	try {
		pathway = await apiGet<AttestationPathwayWithAi>(ctx, `/attestation/${pathwayId}`);
	} catch {
		pathway = null;
	}

	if (!pathway) {
		throw error(404, { code: 'attestation_not_found', message: 'Attestation pathway not found' });
	}

	return { pathway };
};

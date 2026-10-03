import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList, ApiError } from '$lib/server/api';
import type { AttestationPathwayWithAi, GeoCountry } from '$lib/types';

/**
 * Attestation Guide — §97, ADR-088.
 * SSR loads the full 250 primary geo space countries from GET /geo/countries (the
 * geo-aware pattern — destination is the primary data axis). When a ?dest=
 * param is present, the attestation pathways for that destination are fetched
 * server-side so the page renders with content immediately.
 *
 * API: GET /geo/countries — the full 250 primary geo space countries (geo selector).
 *      GET /attestation?dest={space_id}&issuing={space_id}&doc_type={type}
 *          — attestation pathways for a destination.
 *      GET /attestation/{id} — single pathway detail.
 *      GET /attestation/{id}/export/pdf — PDF export metadata.
 *      POST /attestation/{id}/updates — community update (auth).
 *
 * Route: /tools/attestation-guide
 * Ref: §97 (Attestation Guide), ROADMAP-TOOLS 02.06.
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['attestation_guide.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Attestation Guide is not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	// Destination list — the geo-aware selector source (Rule 16).
	// A 404 or attestation_guide_disabled code means the tool is turned off.
	let destinations: GeoCountry[] = [];
	let disabled = false;
	try {
		destinations = (await apiList<GeoCountry>(ctx, '/geo/countries')).items;
	} catch (e) {
		if (e instanceof ApiError && (e.status === 404 || e.code === 'attestation_guide_disabled')) {
			disabled = true;
		}
		destinations = [];
	}

	// If ?dest= is present, fetch attestation pathways for that destination.
	// ?issuing= and ?doc_type= are honoured too so filtered URLs are shareable.
	const destParam = event.url.searchParams.get('dest');
	const initialDestId =
		destParam && destinations.some((d) => d.id === destParam) ? destParam : null;
	const issuingParam = event.url.searchParams.get('issuing');
	const initialIssuingId =
		issuingParam && destinations.some((d) => d.id === issuingParam) ? issuingParam : null;
	const initialDocType = event.url.searchParams.get('doc_type') ?? '';

	let pathways: AttestationPathwayWithAi[] = [];
	let pathwaysTotal = 0;
	if (initialDestId) {
		try {
			const qs = new URLSearchParams({
				dest: initialDestId,
				page_offset: '0',
				page_size: '25'
			});
			if (initialIssuingId) qs.set('issuing', initialIssuingId);
			if (initialDocType) qs.set('doc_type', initialDocType);
			const res = await apiList<AttestationPathwayWithAi>(ctx, `/attestation?${qs}`);
			pathways = res.items;
			pathwaysTotal = res.meta?.total ?? res.items.length;
		} catch {
			pathways = [];
		}
	}

	return {
		destinations,
		pathways,
		pathwaysTotal,
		initialDestId,
		initialIssuingId,
		initialDocType,
		disabled,
		authed
	};
};

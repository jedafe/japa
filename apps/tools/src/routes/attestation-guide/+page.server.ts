import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { AttestationPathwayWithAi, GeoSpaceOption } from '$lib/types';

/**
 * Attestation Guide — §97, ADR-088.
 * SSR loads the destination country list from /spaces?level=country (the
 * geo-aware pattern — destination is the primary data axis). When a ?dest=
 * param is present, the attestation pathways for that destination are fetched
 * server-side so the page renders with content immediately.
 *
 * API: GET /spaces?level=country — destination countries (geo selector).
 *      GET /attestation?dest={space_id}&issuing={space_id}&doc_type={type}
 *          — attestation pathways for a destination.
 *      GET /attestation/{id} — single pathway detail.
 *      GET /attestation/{id}/export/pdf — PDF export metadata.
 *      POST /attestation/{id}/updates — community update (auth).
 *
 * Ref: §97 (Attestation Guide), ROADMAP-TOOLS 02.06.
 */
export const load: PageServerLoad = async (event) => {
	// Gate: skip all backend calls when attestation_guide.enabled is false.
	// Ref: conventions/system-settings-catalog.md, Rule 3.
	const tool = getTool('attestation-guide')!;
	const settings = event.locals.siteSettings;
	if (!isToolEnabled(tool, event.locals.features)) {
		return {
			enabled: false,
			tool,
			destinations: [],
			pathways: [],
			initialDestId: null,
			user: event.locals.user
		};
	}

	const ctx = apiContext(event);

	// Destination list — the geo-aware selector source (Rule 16).
	let destinations: GeoSpaceOption[] = [];
	try {
		destinations = (await apiList<GeoSpaceOption>(ctx, '/spaces?level=country')).items;
	} catch {
		destinations = [];
	}

	// If ?dest= is present, fetch attestation pathways for that destination.
	const destParam = event.url.searchParams.get('dest');
	const initialDestId =
		destParam && destinations.some((d) => d.id === destParam) ? destParam : null;

	let pathways: AttestationPathwayWithAi[] = [];
	if (initialDestId) {
		try {
			pathways = (
				await apiList<AttestationPathwayWithAi>(ctx, `/attestation?dest=${initialDestId}`)
			).items;
		} catch {
			pathways = [];
		}
	}

	return {
		enabled: true,
		tool,
		destinations,
		pathways,
		initialDestId,
		user: event.locals.user
	};
};

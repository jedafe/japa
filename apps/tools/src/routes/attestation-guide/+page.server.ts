import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { loadCountries } from '$lib/server/geo';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { AttestationPathwayWithAi, GeoSpaceOption } from '$lib/types';

/**
 * Attestation Guide — §97, ADR-088.
 * SSR loads the destination country list from GET /geo/countries (the
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
			pathwaysTotal: 0,
			initialDestId: null,
			initialIssuingId: null,
			initialDocType: '',
			user: event.locals.user
		};
	}

	const ctx = apiContext(event);

	// Destination list — the geo-aware selector source (Rule 16).
	let destinations: GeoSpaceOption[] = [];
	try {
		destinations = await loadCountries(ctx);
	} catch {
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
		enabled: true,
		tool,
		destinations,
		pathways,
		pathwaysTotal,
		initialDestId,
		initialIssuingId,
		initialDocType,
		user: event.locals.user
	};
};

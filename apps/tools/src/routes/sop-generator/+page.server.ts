import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import { loadCountries } from '$lib/server/geo';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { GeoSpaceOption, SopTemplate, SopSample, SopDraft, SopQuota } from '$lib/types';

/**
 * AI SOP Generator — §110, ADR-121.
 * SSR loads templates + samples (public) and the quota + drafts (auth).
 * Generation is async — POST /sop/generate returns 202 + draft_id; the
 * client polls GET /sop/drafts/:id until status flips to 'generated'.
 *
 * API: GET /sop/templates?dest=&purpose= — public templates.
 *      GET /sop/samples?dest=&level=&field= — public samples.
 *      GET /sop/samples/:id — sample detail (public).
 *      GET /sop/quota — remaining monthly generations (auth).
 *      POST /sop/generate — enqueue sop:generate (auth, 202).
 *      GET/PATCH/DELETE /sop/drafts/* — draft CRUD (auth owner).
 *
 * Ref: §110 (SOP Generator), ROADMAP-TOOLS 06.04, ADR-042 (flag → 404).
 */
export const load: PageServerLoad = async (event) => {
	const tool = getTool('sop-generator')!;
	if (!isToolEnabled(tool, event.locals.features)) {
		return {
			enabled: false,
			tool,
			destinations: [],
			templates: [],
			samples: [],
			quota: null,
			drafts: [],
			user: event.locals.user
		};
	}

	const ctx = apiContext(event);

	let destinations: GeoSpaceOption[] = [];
	try {
		destinations = await loadCountries(ctx);
	} catch {
		destinations = [];
	}

	let templates: SopTemplate[] = [];
	try {
		templates = (await apiList<SopTemplate>(ctx, '/sop/templates')).items;
	} catch {
		templates = [];
	}

	let samples: SopSample[] = [];
	try {
		samples = (await apiList<SopSample>(ctx, '/sop/samples')).items;
	} catch {
		samples = [];
	}

	// Auth-only: quota + drafts.
	let quota: SopQuota | null = null;
	let drafts: SopDraft[] = [];
	if (event.locals.user) {
		try {
			quota = await apiGet<SopQuota>(ctx, '/sop/quota');
		} catch {
			quota = null;
		}
		try {
			drafts = (await apiList<SopDraft>(ctx, '/sop/drafts')).items;
		} catch {
			drafts = [];
		}
	}

	return {
		enabled: true,
		tool,
		destinations,
		templates,
		samples,
		quota,
		drafts,
		user: event.locals.user
	};
};

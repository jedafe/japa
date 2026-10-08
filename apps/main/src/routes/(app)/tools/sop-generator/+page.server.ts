import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { GeoCountry, GeoSpaceOption, SopTemplate, SopSample, SopField, SopDraft, SopQuota } from '$lib/types';

/**
 * AI SOP Generator at /tools/sop-generator (§110, ADR-121).
 * SSR loads templates + samples (public) and the quota + drafts (auth).
 * Generation is async — POST /sop/generate returns 202 + draft_id; the
 * client polls GET /sop/drafts/:id until status flips to 'generated'.
 *
 * API: GET /sop/templates — public templates.
 *      GET /sop/samples — public samples.
 *      GET /sop/fields — curated field-of-study filter options.
 *      GET /sop/quota — remaining monthly generations (auth).
 *      POST /sop/generate — enqueue sop:generate (auth, 202).
 *      GET/PATCH/DELETE /sop/drafts/* — draft CRUD (auth owner).
 *
 * Ref: japaconnect-engineering/sections/110-sop-generator.md, ADR-042 (flag → 404).
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['sop_generator.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'SOP Generator is not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	let destinations: GeoSpaceOption[] = [];
	try {
		const { items } = await apiList<GeoCountry>(ctx, '/geo/countries');
		destinations = items.map((c) => ({ id: c.id, name: c.name, slug: c.slug, code: c.geo_code ?? null }));
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

	let fields: SopField[] = [];
	try {
		fields = (await apiList<SopField>(ctx, '/sop/fields')).items;
	} catch {
		fields = [];
	}

	let quota: SopQuota | null = null;
	let drafts: SopDraft[] = [];
	if (authed) {
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

	return { destinations, templates, samples, fields, quota, drafts, authed };
};

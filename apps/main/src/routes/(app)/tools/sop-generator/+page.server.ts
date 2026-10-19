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

	// All six SSR fetches are independent — run them concurrently.
	const destinationsPromise = apiList<GeoCountry>(ctx, '/geo/countries')
		.then(({ items }) =>
			items.map((c) => ({ id: c.id, name: c.name, slug: c.slug, code: c.geo_code ?? null }))
		)
		.catch(() => [] as GeoSpaceOption[]);
	const templatesPromise = apiList<SopTemplate>(ctx, '/sop/templates')
		.then((r) => r.items)
		.catch(() => [] as SopTemplate[]);
	const samplesPromise = apiList<SopSample>(ctx, '/sop/samples')
		.then((r) => r.items)
		.catch(() => [] as SopSample[]);
	const fieldsPromise = apiList<SopField>(ctx, '/sop/fields')
		.then((r) => r.items)
		.catch(() => [] as SopField[]);
	const quotaPromise = authed
		? apiGet<SopQuota>(ctx, '/sop/quota').catch(() => null)
		: Promise.resolve(null);
	const draftsPromise = authed
		? apiList<SopDraft>(ctx, '/sop/drafts')
				.then((r) => r.items)
				.catch(() => [] as SopDraft[])
		: Promise.resolve([] as SopDraft[]);

	const [destinations, templates, samples, fields, quota, drafts] = await Promise.all([
		destinationsPromise,
		templatesPromise,
		samplesPromise,
		fieldsPromise,
		quotaPromise,
		draftsPromise
	]);

	return { destinations, templates, samples, fields, quota, drafts, authed };
};

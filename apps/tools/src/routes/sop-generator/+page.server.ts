import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import { loadCountries } from '$lib/server/geo';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { GeoSpaceOption, SopTemplate, SopSample, SopField, SopDraft, SopQuota } from '$lib/types';

/**
 * AI SOP Generator — §110, ADR-121.
 * SSR loads templates + samples (public) and the quota + drafts (auth).
 * Generation is async — POST /sop/generate returns 202 + draft_id; the
 * client polls GET /sop/drafts/:id until status flips to 'generated'.
 *
 * API: GET /sop/templates?dest=&purpose= — public templates.
 *      GET /sop/samples?dest=&level=&field= — public samples.
 *      GET /sop/samples/:id — sample detail (public).
 *      GET /sop/fields — curated field-of-study filter options (public).
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
			fields: [],
			quota: null,
			drafts: [],
			user: event.locals.user
		};
	}

	const ctx = apiContext(event);

	// All datasets are independent — one wave. Each call degrades to its
	// empty fallback on failure so a broken endpoint never blanks the page.
	const authed = !!event.locals.user;
	const [destinations, templates, samples, fields, quota, drafts] = await Promise.all([
		loadCountries(ctx).catch(() => [] as GeoSpaceOption[]),
		apiList<SopTemplate>(ctx, '/sop/templates')
			.then((r) => r.items)
			.catch(() => [] as SopTemplate[]),
		apiList<SopSample>(ctx, '/sop/samples')
			.then((r) => r.items)
			.catch(() => [] as SopSample[]),
		apiList<SopField>(ctx, '/sop/fields')
			.then((r) => r.items)
			.catch(() => [] as SopField[]),
		// Auth-only: quota + drafts.
		authed
			? apiGet<SopQuota>(ctx, '/sop/quota').catch(() => null)
			: Promise.resolve(null),
		authed
			? apiList<SopDraft>(ctx, '/sop/drafts')
					.then((r) => r.items)
					.catch(() => [] as SopDraft[])
			: Promise.resolve([] as SopDraft[])
	]);

	return {
		enabled: true,
		tool,
		destinations,
		templates,
		samples,
		fields,
		quota,
		drafts,
		user: event.locals.user
	};
};

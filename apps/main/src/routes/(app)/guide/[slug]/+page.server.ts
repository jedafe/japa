import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { loadContentArticle } from '#lib/server/content-article.js';
import { contentHref } from '#lib/utils/contentLink.js';
import { apiContext, apiGet, apiList, ApiError } from '#lib/server/api.js';
import { DOC_TYPES } from '#lib/attestation/steps.js';
import type { GeoCountry, Scholarship } from '#lib/types/index.js';

/**
 * Guide single page at /guide/{slug} — the canonical detail route for
 * content_type 'guide' (§14, ADR-058 route families). Shares the generic
 * content loader; a non-guide slug under /guide is redirected to its own
 * canonical route.
 *
 * §73 integration: when the guide is also a scholarship (its content id is
 * the scholarship guide_id), load the matching Scholarship so the page can
 * render a "View Scholarship" CTA.
 */
export const load: PageServerLoad = async (event) => {
	const data = await loadContentArticle(event);

	if (data.article.content_type !== 'guide') {
		throw redirect(301, `${contentHref(data.article)}${event.url.search}`);
	}

	const ctx = apiContext(event);
	let scholarship: { guide_id: string; slug: string; title: string } | null = null;
	try {
		const res = await apiGet<Scholarship>(ctx, `/scholarships/${data.article.id}`);
		scholarship = {
			guide_id: res.guide_id,
			slug: res.slug,
			title: res.title
		};
	} catch (err) {
		// 404 simply means this guide is not also in the scholarship finder.
		if (!(err instanceof ApiError && err.status === 404)) {
			// Degrade silently — the guide page should still render.
		}
	}

	// §97 attestation integration — when the guide is in the attestation
	// category, resolve its geo_countries field values (geo codes) into
	// country names server-side so the details card and CTA text are
	// human-readable in SSR output (matters for SEO — crawlers see names,
	// not "NG"), and build the cross-link href into the interactive tool.
	// An optional pathway_id is validated against the public endpoint so a
	// stale/deleted pathway falls back to the corridor-filtered tool
	// landing instead of a 404.
	const isAttestationGuide =
		data.article.content_type === 'guide' && data.article.guide_category === 'attestation';
	const fv = (data.article.field_values ?? {}) as Record<string, unknown>;
	const uuidRe = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
	const corridorHref = (): string => {
		const q = new URLSearchParams();
		if (typeof fv.destination_country === 'string' && fv.destination_country.trim()) {
			q.set('dest_geo', fv.destination_country.trim().toUpperCase());
		}
		if (typeof fv.issuing_country === 'string' && fv.issuing_country.trim()) {
			q.set('issuing_geo', fv.issuing_country.trim().toUpperCase());
		}
		if (
			typeof fv.document_type === 'string' &&
			(DOC_TYPES as readonly { value: string }[]).some((d) => d.value === fv.document_type)
		) {
			q.set('doc_type', fv.document_type);
		}
		const s = q.toString();
		return `/tools/attestation-guide${s ? `?${s}` : ''}`;
	};

	let geoCountries: GeoCountry[] = [];
	let attestationToolHref: string | null = null;
	if (isAttestationGuide) {
		const [{ features }, countries] = await Promise.all([
			event.parent(),
			apiList<GeoCountry>(ctx, '/geo/countries').then((r) => r.items).catch(() => [] as GeoCountry[])
		]);
		geoCountries = countries;

		if (features['attestation_guide.enabled']) {
			const pid = typeof fv.pathway_id === 'string' ? fv.pathway_id.trim() : '';
			let linked = false;
			if (uuidRe.test(pid)) {
				try {
					await apiGet(ctx, `/attestation/${pid}`);
					attestationToolHref = `/tools/attestation-guide/${pid}`;
					linked = true;
				} catch {
					// Stale/unpublished pathway id — fall back to the corridor link.
				}
			}
			if (!linked) attestationToolHref = corridorHref();
		}
	}

	return { ...data, scholarship, geoCountries, attestationToolHref };
};

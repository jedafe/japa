import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { loadContentArticle } from '#lib/server/content-article.js';
import { contentHref } from '#lib/utils/contentLink.js';
import { apiContext, apiGet, apiList, ApiError } from '#lib/server/api.js';
import { DOC_TYPES } from '#lib/attestation/steps.js';
import type { AttestationPathwayWithAi, GeoCountry, Scholarship } from '#lib/types/index.js';

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
	let ecaToolHref: string | null = null;
	let credentialToolHref: string | null = null;
	let housingToolHref: string | null = null;
	const guideCategory = data.article.guide_category;
	const needsGeo =
		guideCategory === 'attestation' ||
		guideCategory === 'eca' ||
		guideCategory === 'credential' ||
		guideCategory === 'accommodation';
	const { features } = await event.parent();
	if (needsGeo) {
		geoCountries = await apiList<GeoCountry>(ctx, '/geo/countries')
			.then((r) => r.items)
			.catch(() => [] as GeoCountry[]);
	}

	// geo code → space id — the tool landings take ?dest=<space id> while
	// editorial field_values store geo_countries codes (§97 pattern).
	const spaceIdOf = (geo: unknown) =>
		typeof geo === 'string' && geo.trim()
			? (geoCountries.find((c) => c.geo_code?.toUpperCase() === geo.trim().toUpperCase())?.id ?? null)
			: null;

	if (isAttestationGuide) {
		if (features['attestation_guide.enabled']) {
			const pid = typeof fv.pathway_id === 'string' ? fv.pathway_id.trim() : '';
			let linked = false;
			if (uuidRe.test(pid)) {
				try {
					await apiGet(ctx, `/attestation/${pid}`);
					attestationToolHref = `/tools/attestation-guide/${pid}`;
					linked = true;
				} catch {
					// Stale/unpublished pathway id — fall through to resolution.
				}
			}
			// Auto-resolution: the document_type + issuing/destination country
			// combination is the pathway's natural key, so a guide links to its
			// pathway without anyone filling pathway_id manually. A single
			// unambiguous match deep-links straight to the pathway; anything
			// else falls back to the corridor-filtered tool landing.
			if (!linked) {
				const docType =
					typeof fv.document_type === 'string' &&
					(DOC_TYPES as readonly { value: string }[]).some(
						(d) => d.value === fv.document_type
					)
						? fv.document_type
						: '';
				const destId = spaceIdOf(fv.destination_country);
				const issId = spaceIdOf(fv.issuing_country);
				if (docType && destId && issId) {
					try {
						const qs = new URLSearchParams({
							dest: destId,
							issuing: issId,
							doc_type: docType,
							page_size: '2'
						});
						const res = await apiList<AttestationPathwayWithAi>(
							ctx,
							`/attestation?${qs}`
						);
						if (res.items.length === 1) {
							attestationToolHref = `/tools/attestation-guide/${res.items[0].id}`;
							linked = true;
						}
					} catch {
						// Lookup failure — fall back to the corridor link.
					}
				}
			}
			if (!linked) attestationToolHref = corridorHref();
		}
	}

	// §109 — eca-category guides deep-link into the ECA tool on its natural
	// key (destination × provider slug × purpose). The landing accepts
	// ?dest=<space id>&provider=<slug>&purpose=<enum>, so no extra fetch is
	// needed beyond the geo code → space id resolution.
	if (guideCategory === 'eca' && features['eca_guide.enabled']) {
		const q = new URLSearchParams();
		const destId = spaceIdOf(fv.destination_country);
		if (destId) q.set('dest', destId);
		if (typeof fv.provider === 'string' && fv.provider.trim()) {
			q.set('provider', fv.provider.trim().toLowerCase());
		}
		if (
			typeof fv.purpose === 'string' &&
			['immigration', 'study', 'employment', 'professional_licensure'].includes(fv.purpose)
		) {
			q.set('purpose', fv.purpose);
		}
		const s = q.toString();
		if (s) ecaToolHref = `/tools/eca-guide${s ? `?${s}` : ''}`;
	}

	// §94 — credential-category guides deep-link into the credential checker
	// on its natural key (profession × issuing × destination). The tool's
	// ?profession= filter takes the profession id while field_values stores
	// the slug, so resolve it against the public professions list.
	if (guideCategory === 'credential' && features['credential_checker.enabled']) {
		const q = new URLSearchParams();
		const destId = spaceIdOf(fv.destination_country);
		const issId = spaceIdOf(fv.issuing_country);
		if (destId) q.set('dest', destId);
		if (issId) q.set('issuing', issId);
		const profSlug = typeof fv.profession === 'string' ? fv.profession.trim().toLowerCase() : '';
		if (profSlug) {
			try {
				const { items: professions } = await apiList<{ id: string; slug: string }>(
					ctx,
					'/credentials/professions'
				);
				const prof = professions.find((p) => p.slug?.toLowerCase() === profSlug);
				if (prof) q.set('profession', prof.id);
			} catch {
				// Lookup failure — land on the corridor-filtered tool page.
			}
		}
		const s = q.toString();
		if (s) credentialToolHref = `/tools/credential-checker${s ? `?${s}` : ''}`;
	}

	// §96 — accommodation-category guides deep-link into the housing tool on
	// its natural key (destination geo code + city + accommodation_type).
	// An explicit guide_id deep-links to the tool detail when the row
	// resolves (validated against the public endpoint); otherwise a single
	// natural-key match deep-links, and anything else lands on the
	// dest+type-filtered tool landing.
	if (guideCategory === 'accommodation' && features['housing_guide.enabled']) {
		const guid = typeof fv.guide_id === 'string' ? fv.guide_id.trim() : '';
		let linked = false;
		if (uuidRe.test(guid)) {
			try {
				await apiGet(ctx, `/housing-guide/${guid}`);
				housingToolHref = `/tools/housing-guide/${guid}`;
				linked = true;
			} catch {
				// Stale/unpublished guide id — fall through to resolution.
			}
		}
		if (!linked) {
			const destId = spaceIdOf(fv.destination_country);
			const accomType =
				typeof fv.accommodation_type === 'string' ? fv.accommodation_type.trim().toLowerCase() : '';
			const cityName = typeof fv.city === 'string' ? fv.city.trim() : '';
			if (destId) {
				try {
					const qs = new URLSearchParams({ dest: destId, page_size: '50' });
					if (accomType) qs.set('type', accomType);
					const res = await apiList<{ id: string; city_space_name?: string | null }>(
						ctx,
						`/housing-guide?${qs}`
					);
					const matches = res.items.filter(
						(g) => (g.city_space_name ?? '').toLowerCase() === cityName.toLowerCase()
					);
					if (matches.length === 1) {
						housingToolHref = `/tools/housing-guide/${matches[0].id}`;
						linked = true;
					}
				} catch {
					// Lookup failure — land on the corridor-filtered tool page.
				}
				if (!linked) {
					const q = new URLSearchParams({ dest: destId });
					if (accomType) q.set('type', accomType);
					housingToolHref = `/tools/housing-guide?${q}`;
				}
			}
		}
	}

	return { ...data, scholarship, geoCountries, attestationToolHref, ecaToolHref, credentialToolHref, housingToolHref };
};

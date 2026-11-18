import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '#lib/server/api.js';
import { ApiError } from '#lib/server/api.js';
import type { ChecklistGuide, ChecklistSelector, GeoCountry } from '#lib/types/index.js';

interface CountryOptionWithCode {
	id: string;
	name: string;
	slug: string;
	code: string;
}

/**
 * Visa Checklist tool page at /tools/visa-checklist (Section 70, ADR-070).
 * SSR-fetches the selector options (origins + destinations with nested visa
 * types) so the 3-step selector renders without a client round-trip.
 *
 * `?guide_id=<uuid>` deep-links straight into a checklist (shared/bookmarked
 * links) — the guide's items (and the signed-in user's progress) are
 * server-loaded so the checklist renders on first paint.
 *
 * Ref: japaconnect-engineering/sections/70-visa-checklist.md
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['checklist.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Visa Document Checklist is not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	let selector: ChecklistSelector | null = null;
	let disabled = false;
	let selectorFailed = false;
	// ADR-064: full country list with ISO geo_code so the origin picker can
	// pin curated origin/destination markets first (siteSettings-driven).
	let countries: CountryOptionWithCode[] = [];
	try {
		const [sel, geoCountries] = await Promise.all([
			apiGet<ChecklistSelector>(ctx, '/checklist/selector'),
			apiGet<GeoCountry[]>(ctx, '/geo/countries').catch(() => [] as GeoCountry[])
		]);
		selector = sel;
		countries = geoCountries.map((c) => ({
			id: c.id,
			name: c.name,
			slug: c.slug,
			code: c.geo_code ?? ''
		}));
	} catch (e) {
		if (e instanceof ApiError && (e.code === 'checklist_disabled' || e.status === 404)) {
			disabled = true;
		} else {
			// Transient/backend failure — the page surfaces a retry state
			// instead of an infinite loading skeleton.
			selectorFailed = true;
		}
	}

	// Deep-linked guide (?guide_id=) — load items, plus the user's progress
	// when signed in.
	let guide: ChecklistGuide | null = null;
	let progressItems: { item_key: string; completed: boolean; notes?: string }[] = [];
	// A failed progress fetch must surface — silently rendering everything
	// unticked looks like lost data.
	let progressFailed = false;
	const guideId = event.url.searchParams.get('guide_id');
	if (guideId && selector && !disabled) {
		try {
			guide = await apiGet<ChecklistGuide>(ctx, `/checklist/${encodeURIComponent(guideId)}`);
		} catch {
			guide = null;
		}
		if (guide && authed) {
			try {
				const prog = await apiGet<{
					items?: { item_key: string; completed: boolean; notes?: string }[];
				}>(ctx, `/checklist/${encodeURIComponent(guideId)}/progress`);
				progressItems = prog?.items ?? [];
			} catch {
				progressFailed = true;
			}
		}
	}

	// A requested guide that didn't resolve (unpublished, wrong category,
	// deleted) must surface a notice — silently dropping to the generic
	// selector leaves shared-link users wondering what happened.
	const guideUnavailable = !!guideId && !guide && !disabled && !selectorFailed;

	// §90 integration: `?route_id=<uuid>` preserves which Visa Route
	// Comparison entry sent the user here — the checklist shows a "back to
	// comparison" chip, and when the route has no published guide yet the
	// selector still backfills from the route's destination + visa
	// category. Best-effort lookup; a stale/deleted route_id hides the
	// chip and prefill.
	let sourceRoute: {
		id: string;
		title: string;
		destination?: string;
		visaCategory?: string;
	} | null = null;
	const routeId = event.url.searchParams.get('route_id');
	if (routeId) {
		try {
			const r = await apiGet<{
				id: string;
				name?: string;
				destination_space_name?: string;
				visa_category?: string;
			}>(ctx, `/visa-routes/${encodeURIComponent(routeId)}`);
			sourceRoute = {
				id: routeId,
				title: r?.name ?? '',
				destination: r?.destination_space_name,
				visaCategory: r?.visa_category
			};
		} catch {
			sourceRoute = null;
		}
	}

	return { selector, disabled, selectorFailed, authed, countries, guide, progressItems, progressFailed, guideUnavailable, sourceRoute };
};

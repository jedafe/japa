import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import { ApiError } from '$lib/server/api';
import type { ChecklistGuide, ChecklistSelector, GeoCountry } from '$lib/types';

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
				progressItems = [];
			}
		}
	}

	return { selector, disabled, selectorFailed, authed, countries, guide, progressItems };
};

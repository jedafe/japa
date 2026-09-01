import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiGetOrNull, apiList } from '$lib/server/api';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { AssessmentForm, AssessmentResult, AssessmentHistoryItem } from '$lib/types';

export interface CountryOption {
	value: string;
	label: string;
}

export interface JourneyOptions {
	purposes: CountryOption[];
	stages: CountryOption[];
	visa_routes: CountryOption[];
}

export interface SpaceOption {
	id: string;
	name: string;
	slug: string;
	geo_code?: string;
}

/**
 * Japa Readiness Assessment — §71, ADR-071.
 * SSR loads the active assessment form + scoring metadata. If a result ID is
 * in the URL (shareable link), load the full result directly. For authenticated
 * users, also load their past results history (§71: "authenticated users get
 * persistent history"). History items are summaries; the full result is
 * fetched client-side when a user opens one.
 *
 * API: GET /assessment — active form + scoring metadata.
 *      GET /assessment/result/:id — full result + roadmap.
 *      GET /assessment/history — user's past result summaries (auth only).
 *
 * Ref: §71 (Readiness Assessment), ROADMAP-TOOLS 02.03.
 */
export const load: PageServerLoad = async (event) => {
	// Gate: disabled features redirect home per ADR-042.
	const tool = getTool('readiness')!;
	if (!isToolEnabled(tool, event.locals.features)) {
		throw redirect(302, '/');
	}

	const ctx = apiContext(event);

	let form: AssessmentForm | null = null;
	try {
		form = await apiGet<AssessmentForm>(ctx, '/assessment');
	} catch {
		form = null;
	}

	// If a result ID is in the URL, load it directly (shareable results page).
	const resultId = event.url.searchParams.get('result');
	let result: AssessmentResult | null = null;
	if (resultId) {
		try {
			result = await apiGetOrNull<AssessmentResult>(ctx, `/assessment/result/${resultId}`);
		} catch {
			result = null;
		}
	}

	// Load the authenticated user's past results (§71 history).
	// Ref: §71 — "authenticated users get persistent history".
	let history: AssessmentHistoryItem[] = [];
	if (event.locals.user) {
		try {
			const res = await apiList<AssessmentHistoryItem>(ctx, '/assessment/history?limit=50');
			history = res.items;
		} catch {
			history = [];
		}
	}

	// Journey option lists and canonical country options are fetched separately
	// from the form definition so country selectors are never hardcoded.
	let journeyOptions: JourneyOptions = { purposes: [], stages: [], visa_routes: [] };
	let countryOptions: CountryOption[] = [];

	try {
		journeyOptions = await apiGet<JourneyOptions>(ctx, '/feed/journey-options');
	} catch {
		journeyOptions = { purposes: [], stages: [], visa_routes: [] };
	}

	try {
		const allSpaces: SpaceOption[] = [];
		let offset = 0;
		while (true) {
			const { items, meta } = await apiList<SpaceOption>(
				ctx,
				`/spaces?level=country&sort=name&page_size=100&page_offset=${offset}`
			);
			if (!items.length) break;
			allSpaces.push(...items);
			if (meta?.total && allSpaces.length >= meta.total) break;
			offset += items.length;
		}
		countryOptions = allSpaces
			.filter((s) => s.geo_code)
			.map((s) => ({ value: s.geo_code as string, label: s.name }))
			.sort((a, b) => a.label.localeCompare(b.label));
	} catch {
		countryOptions = [];
	}

	return {
		enabled: true,
		tool,
		form,
		result,
		history,
		user: event.locals.user,
		journeyOptions,
		countryOptions
	};
};

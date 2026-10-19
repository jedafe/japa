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

	// If a result ID is in the URL, load it directly (shareable results page).
	const resultId = event.url.searchParams.get('result');
	const authed = !!event.locals.user;

	// Form, shared result, history, journey options, and the country list are
	// all independent — one wave, each degrading to its empty fallback.
	const [form, result, history, journeyOptions, countryOptions] = await Promise.all([
		apiGet<AssessmentForm>(ctx, '/assessment').catch(() => null),
		resultId
			? apiGetOrNull<AssessmentResult>(ctx, `/assessment/result/${resultId}`).catch(() => null)
			: Promise.resolve(null),
		authed
			? apiList<AssessmentHistoryItem>(ctx, '/assessment/history?limit=50')
					.then((r) => r.items)
					.catch(() => [] as AssessmentHistoryItem[])
			: Promise.resolve([] as AssessmentHistoryItem[]),
		apiGet<JourneyOptions>(ctx, '/feed/journey-options').catch(() => ({
			purposes: [] as CountryOption[],
			stages: [] as CountryOption[],
			visa_routes: [] as CountryOption[]
		})),
		(async () => {
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
			return allSpaces
				.filter((s) => s.geo_code)
				.map((s) => ({ value: s.geo_code as string, label: s.name }))
				.sort((a, b) => a.label.localeCompare(b.label));
		})().catch(() => [] as CountryOption[])
	]);

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

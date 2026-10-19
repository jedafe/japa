import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import { ApiError } from '$lib/server/api';
import type { AssessmentForm } from '$lib/types';

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
 * Readiness Assessment page at /tools/readiness (Section 71, ADR-071).
 * SSR-fetches the active assessment form so the multi-section form renders
 * without a client round-trip. For authenticated users, also fetches score
 * history (/assessment/history) so the page can render the score history
 * LineChart (Phase 33.02).
 *
 * Ref: japaconnect-engineering/sections/71-readiness-assessment.md
 */
export interface AssessmentHistoryEntry {
	id: string;
	total_score: number;
	threshold_label?: string;
	submission_id?: string;
	created_at: string;
}

export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['assessment.enabled']) {
		// ADR-042: disabled features redirect to / (not 403, not an error page).
		throw redirect(302, '/');
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	let disabled = false;
	const loadCountries = async (): Promise<CountryOption[]> => {
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
	};

	const [form, history, journeyOptions, countryOptions] = await Promise.all([
		apiGet<AssessmentForm>(ctx, '/assessment').catch((e) => {
			if (e instanceof ApiError && (e.code === 'assessment_disabled' || e.code === 'no_active_assessment' || e.status === 404)) {
				disabled = true;
			}
			return null;
		}),
		// Phase 33.02 — fetch the authenticated user's score history so the
		// page can render the score history LineChart. Best-effort: guests and
		// any backend error degrade to an empty list.
		authed
			? apiList<AssessmentHistoryEntry>(ctx, '/assessment/history?limit=50')
					.then((r) => r.items)
					.catch(() => [] as AssessmentHistoryEntry[])
			: Promise.resolve([] as AssessmentHistoryEntry[]),
		// Journey field option lists. The form uses dynamic custom-field
		// options for stage/visa/purpose, but country selectors are populated
		// from the canonical Spaces directory (ADR-064) so the list is never
		// hardcoded.
		apiGet<JourneyOptions>(ctx, '/feed/journey-options').catch(
			() => ({ purposes: [], stages: [], visa_routes: [] }) as JourneyOptions
		),
		loadCountries().catch(() => [] as CountryOption[])
	]);

	return { form, disabled, authed, history, journeyOptions, countryOptions };
};

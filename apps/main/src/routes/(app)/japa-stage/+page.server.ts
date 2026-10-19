// Ref: japaconnect-engineering/sections/87-japa-stage-profiles.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { MemberStage, JapaStageOption, JapaIntent, GeoCountry } from '$lib/types';

const PAGE_SIZE = 20;

/**
 * Japa Stage directory at /japa-stage — lists public member profiles grouped
 * by their japa_stage. Pass-through ?stage filter and ?page to the backend's
 * GET /members/by-stage endpoint.
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['japa_stage.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Japa Stage is not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;
	const stage = event.url.searchParams.get('stage') ?? '';
	const intent = event.url.searchParams.get('intent') ?? '';
	const dest = event.url.searchParams.get('dest') ?? '';
	const page = Number(event.url.searchParams.get('page') ?? '1') || 1;

	let members: MemberStage[] = [];
	let totalPages = 1;
	let loadError = false;
	const params = new URLSearchParams({ page_size: String(PAGE_SIZE), page: String(page) });
	if (stage) params.set('stage', stage);
	if (intent) params.set('intent', intent);
	if (dest) params.set('dest', dest);

	// Members list, country filter options, and stage/intent option lists are
	// independent — fetch concurrently.
	const [membersResult, countries, stages, intents] = await Promise.all([
		apiList<MemberStage>(ctx, `/members/by-stage?${params.toString()}`).then(
			({ items, meta }) => ({ items, totalPages: meta?.total_pages ?? 1, loadError: false }),
			() => ({ items: [] as MemberStage[], totalPages: 0, loadError: true })
		),
		apiList<GeoCountry>(ctx, '/geo/countries')
			.then((r) => r.items)
			.catch(() => [] as GeoCountry[]),
		apiGet<{ value: string; label: string; intents?: JapaIntent[] }[]>(ctx, '/japa-stage/stages')
			.then((list) => (list ?? []).map((o) => ({ value: o.value, intents: o.intents }) as JapaStageOption))
			.catch(() => [] as JapaStageOption[]),
		apiGet<{ value: string; label: string }[]>(ctx, '/japa-stage/intents')
			.then((list) => (list ?? []).map((o) => o.value as JapaIntent))
			.catch(() => [] as JapaIntent[])
	]);

	members = membersResult.items;
	totalPages = membersResult.totalPages;
	loadError = membersResult.loadError;

	return {
		members,
		authed,
		countries,
		stages,
		intents,
		pageSize: PAGE_SIZE,
		currentPage: page,
		totalPages,
		loadError,
		filters: { stage, intent, dest }
	};
};

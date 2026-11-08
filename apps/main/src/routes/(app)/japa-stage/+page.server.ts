// Ref: japaconnect-engineering/sections/87-japa-stage-profiles.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '#lib/server/api.js';
import type { MemberStage, JapaStageOption, JapaStageStats, JapaIntent, GeoCountry, TrendingContent } from '#lib/types/index.js';

const PAGE_SIZE = 20;
const SORT_KEYS = new Set(['recent', 'stage_updated', 'name']);

/**
 * Japa Stage discovery hub at /japa-stage — hero, stage counts, search and
 * filters, enriched member cards from GET /members/by-stage, plus stats
 * (GET /japa-stage/stats) and trending discussions (GET /trending/content)
 * for the right-sidebar widgets. Pass-through ?stage&intent&dest&q&sort&page.
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
	const gender = event.url.searchParams.get('gender') ?? '';
	const q = (event.url.searchParams.get('q') ?? '').trim();
	const sortParam = event.url.searchParams.get('sort') ?? '';
	const sort = SORT_KEYS.has(sortParam) ? sortParam : 'recent';
	const page = Number(event.url.searchParams.get('page') ?? '1') || 1;

	const params = new URLSearchParams({ page_size: String(PAGE_SIZE), page: String(page) });
	if (stage) params.set('stage', stage);
	if (intent) params.set('intent', intent);
	if (dest) params.set('dest', dest);
	if (gender) params.set('gender', gender);
	if (q) params.set('q', q);
	if (sort !== 'recent') params.set('sort', sort);

	// Members list, stats, trending discussions, and the filter option lists
	// are independent — fetch concurrently. Stats/discussions degrade to
	// null/[] so a widget failure never blanks the directory.
	const [membersResult, stats, discussions, countries, stages, intents] = await Promise.all([
		apiList<MemberStage>(ctx, `/members/by-stage?${params.toString()}`).then(
			({ items, meta }) => ({ items, totalPages: meta?.total_pages ?? 1, total: meta?.total ?? 0, loadError: false }),
			() => ({ items: [] as MemberStage[], totalPages: 0, total: 0, loadError: true })
		),
		apiGet<JapaStageStats>(ctx, '/japa-stage/stats').catch(() => null),
		apiList<TrendingContent>(ctx, '/trending/content?page_size=8')
			.then((r) => r.items)
			.catch(() => [] as TrendingContent[]),
		// The dest filter only needs id/name/geo_code — the /geo/countries
		// rows carry ~300-char descriptions plus counters, and serializing
		// all ~250 of them added ~120KB to the hydration payload.
		apiList<GeoCountry>(ctx, '/geo/countries')
			.then((r) => r.items.map((c) => ({ id: c.id, name: c.name, geo_code: c.geo_code })))
			.catch(() => [] as Pick<GeoCountry, 'id' | 'name' | 'geo_code'>[]),
		apiGet<{ value: string; label: string; intents?: JapaIntent[] }[]>(ctx, '/japa-stage/stages')
			.then((list) => (list ?? []).map((o) => ({ value: o.value, intents: o.intents }) as JapaStageOption))
			.catch(() => [] as JapaStageOption[]),
		apiGet<{ value: string; label: string }[]>(ctx, '/japa-stage/intents')
			.then((list) => (list ?? []).map((o) => o.value as JapaIntent))
			.catch(() => [] as JapaIntent[])
	]);

	// Staff gate for the inline guide editor in the "How it works"
	// slide-over — super_admin group bypasses, else admin.settings.* node.
	const isSuperAdmin = (event.locals.user?.groups ?? []).some(
		(g) => g === 'Super Admin' || g === 'super_admin'
	);
	let canManage = isSuperAdmin;
	if (event.locals.user && !canManage) {
		try {
			const perms = await apiGet<{ permissions: string[] }>(ctx, '/me/permissions');
			canManage = (perms?.permissions ?? []).some((p) => p.startsWith('admin.settings'));
		} catch {
			canManage = false;
		}
	}

	return {
		members: membersResult.items,
		total: membersResult.total,
		authed,
		canManage,
		stats,
		discussions,
		countries,
		stages,
		intents,
		pageSize: PAGE_SIZE,
		currentPage: page,
		totalPages: membersResult.totalPages,
		loadError: membersResult.loadError,
		filters: { stage, intent, dest, gender, q, sort }
	};
};

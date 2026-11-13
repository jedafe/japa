import { error } from '@sveltejs/kit';

import { apiContext, apiGet, apiList } from '#lib/server/api.js';
import { familyForSlug } from '#lib/components/compare/compareFamilies.js';
import type { ComparisonType, ComparisonTypeConfig, JCEComparisonSummary } from '#lib/types/jce.js';
import type { ListMeta } from '#lib/types/index.js';
import type { PageServerLoad } from './$types';

function parseConfig(raw: unknown): ComparisonTypeConfig | null {
	if (!raw) return null;
	if (typeof raw === 'object') return raw as ComparisonTypeConfig;
	if (typeof raw !== 'string') return null;
	const decoded = Buffer.isBuffer(raw) ? raw : Buffer.from(raw, 'base64').toString('utf-8');
	try {
		return JSON.parse(decoded) as ComparisonTypeConfig;
	} catch {
		return null;
	}
}

function enrichTypes(types: ComparisonType[]): ComparisonType[] {
	return types.map((t) => ({
		...t,
		config: parseConfig(t.config as unknown) ?? undefined
	}));
}

function mapComparisonTypes(
	comparisons: JCEComparisonSummary[],
	types: ComparisonType[]
): JCEComparisonSummary[] {
	const byID = new Map<string, ComparisonType>();
	for (const t of types) byID.set(t.id, t);
	return comparisons.map((c) => {
		const type = c.type_id ? byID.get(c.type_id) : undefined;
		return type ? { ...c, type_slug: type.slug, type_name: type.name } : c;
	});
}

export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	const ctx = apiContext(event);
	const family = familyForSlug(event.params.category);
	if (!family) throw error(404, 'Unknown comparison category', { code: 'not_found' });

	let allTypes: ComparisonType[] = [];
	try {
		const res = await apiGet<ComparisonType[]>(ctx, '/jce/comparison-types?page_size=100');
		allTypes = enrichTypes(res ?? []);
	} catch {
		allTypes = [];
	}

	// Member types drive browse + facet scoping — can_create is a picker
	// concern, so unlisted-but-existing comparisons still browse. The mentors
	// gate mirrors /compare's landing filter (feature off → never surfaced).
	let memberTypes = allTypes.filter((t) => t.config?.family === family.key);
	if (features['mentorship.comparison'] !== true) {
		memberTypes = memberTypes.filter((t) => t.slug !== 'mentors');
	}
	if (memberTypes.length === 0) throw error(404, 'Unknown comparison category', { code: 'not_found' });

	const sp = event.url.searchParams;
	const sub = sp.get('sub') ?? '';
	const q = sp.get('q') ?? '';
	const theme = sp.get('theme') ?? '';
	const sort = sp.get('sort') ?? 'popularity';
	const page = Math.max(1, parseInt(sp.get('page') ?? '1', 10) || 1);
	const pageSize = Math.min(96, Math.max(1, parseInt(sp.get('page_size') ?? '12', 10) || 12));

	// `sub` narrows to one member type; invalid subs widen back to the family.
	const validSub = sub && memberTypes.some((t) => t.slug === sub) ? sub : '';
	const activeSlugs = validSub ? [validSub] : memberTypes.map((t) => t.slug);
	const params = new URLSearchParams({
		filter_type: activeSlugs.join(','),
		filter_depth: 'detailed',
		filter_status: 'published',
		filter_visibility: 'public',
		sort,
		page: String(page),
		page_size: String(pageSize),
		facet: 'types,themes',
		facet_scope: memberTypes.map((t) => t.slug).join(',')
	});
	if (q) params.set('q', q);
	if (theme) params.set('filter_theme', theme);

	let items: JCEComparisonSummary[] = [];
	let meta: ListMeta = { total: 0, page: 1, limit: pageSize };
	let loadError = '';
	try {
		const res = await apiList<JCEComparisonSummary>(ctx, `/jce/comparisons?${params}`);
		items = res.items;
		meta = res.meta ?? meta;
	} catch {
		loadError = 'jce.comparisons.errors.load';
	}

	return {
		// Serializable only — the page resolves the full CompareFamily
		// (its icon component isn't serializable across the load boundary).
		familySlug: family.slug,
		memberTypes,
		creatableTypes: memberTypes.filter((t) => t.can_create !== false),
		items: mapComparisonTypes(items, allTypes),
		meta,
		sub: validSub,
		filters: { q, theme, sort, page, page_size: pageSize },
		loadError,
		user: event.locals.user
	};
};

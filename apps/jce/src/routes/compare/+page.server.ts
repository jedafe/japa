import { apiContext, apiGet, ApiError, API_V1 } from '$lib/server/api';
import type { ComparisonType, ComparisonTypeConfig, ResolvedItem } from '$lib/types/jce';
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

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const searchParams = event.url.searchParams;

	let types: ComparisonType[] = [];
	try {
		const res = await apiGet<ComparisonType[]>(ctx, '/jce/comparison-types');
		types = (res ?? []).map((t) => ({
			...t,
			config: parseConfig(t.config as unknown) ?? undefined
		}));
	} catch (err) {
		if (err instanceof ApiError && err.status === 403) {
			types = [];
		} else {
			throw err;
		}
	}

	const preselectedType = searchParams.get('type') ?? '';
	const compareToken = searchParams.get('compare_token') ?? '';

	let preselectedItems: ResolvedItem[] = [];
	const preselect = searchParams.get('preselect') ?? '';
	if (preselect) {
		const ids = preselect.split(',').filter(Boolean);
		const type = types.find((t) => t.slug === preselectedType);
		const refType = (type?.config?.item_ref_types?.[0] as string) ?? 'space';
		for (const id of ids.slice(0, 4)) {
			try {
				const item = await apiGet<ResolvedItem>(ctx, `/jce/entities/${refType}/${id}`);
				if (item) preselectedItems.push(item);
			} catch {
				// Ignore resolution failures for malformed preselect values.
			}
		}
	}

	return {
		types,
		preselectedType,
		preselectedItems,
		compareToken,
		user: event.locals.user,
		apiBase: API_V1,
		mainOrigin: (event.locals.siteSettings?.['jce.main_origin'] as string) ?? 'http://localhost:3000'
	};
};

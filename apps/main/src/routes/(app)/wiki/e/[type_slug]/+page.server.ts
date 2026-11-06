// Ref: japaconnect-engineering/sections/80-destination-wiki.md (Entity Directory amendment)
import { error } from '@sveltejs/kit';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import { hasPerm } from '$lib/server/auth';
import type { PageServerLoad } from './$types';

export interface EntityDirectoryItem {
	id: string;
	slug: string;
	name: string;
	discriminator?: string;
	href: string;
}

interface EntitiesResponse {
	items: EntityDirectoryItem[];
	total: number;
	kind: string;
	label: string;
}

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const kind = event.params.type_slug;
	const q = event.url.searchParams.get('q') ?? '';
	const page = Math.max(1, parseInt(event.url.searchParams.get('page') ?? '1', 10) || 1);
	const limit = 24;
	const offset = (page - 1) * limit;

	const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
	if (q) params.set('q', q);

	let res: EntitiesResponse = { items: [], total: 0, kind, label: kind };
	try {
		res = (await apiGet<EntitiesResponse>(ctx, `/wiki/entity-kinds/${kind}/entities?${params}`)) ?? res;
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) throw error(404);
	}

	const creatableKinds = ['product', 'program', 'course', 'embassy', 'bank', 'visa_route', 'scholarship'];
	return {
		kind: res.kind ?? kind,
		slug: kind,
		label: res.label ?? kind,
		q,
		page,
		limit,
		items: res.items ?? [],
		total: res.total ?? 0,
		canCreate: hasPerm(event.locals.user?.permissions, 'wiki.edit') && creatableKinds.includes(res.kind ?? kind)
	};
};

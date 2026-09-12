import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { Source, SourceItemEnvelope, SourceMeta } from '$lib/types/sources';

const PAGE_SIZE = 24;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;
	if (!slug) {
		error(404, { code: 'not_found', message: 'Source not found' });
	}

	const source = await apiGet<Source>(ctx, `/sources/${encodeURIComponent(slug)}`).catch(() => {
		error(404, { code: 'not_found', message: 'Source not found' });
	});

	const sourceMeta = await apiGet<SourceMeta>(ctx, '/sources/meta').catch(() => ({
		categories: [],
		source_types: [],
		entity_types: [],
		sorts: [],
		item_sorts: []
	}));

	const entityTypes = sourceMeta.entity_types ?? [];
	const validEntityTypes = new Set(['', ...entityTypes.map((e) => e.value)]);

	const rawEntityType = event.url.searchParams.get('entity_type') ?? '';
	const entityType = validEntityTypes.has(rawEntityType) ? rawEntityType : '';

	const itemSorts = sourceMeta.item_sorts ?? [];
	const validSorts = new Set(itemSorts.map((s) => s.value));
	const rawSort = event.url.searchParams.get('sort') ?? 'latest';
	const sort = validSorts.has(rawSort) ? rawSort : (itemSorts[0]?.value ?? 'latest');

	const params = new URLSearchParams();
	if (entityType) params.set('entity_type', entityType);
	params.set('sort', sort);
	params.set('page_size', String(PAGE_SIZE));
	params.set('page_offset', '0');

	const feed = await apiGet<SourceItemEnvelope>(
		ctx,
		`/sources/${encodeURIComponent(slug)}/items?${params.toString()}`
	).catch(() => ({
		items: [],
		meta: { page_size: PAGE_SIZE, page_offset: 0, total: 0 },
		sort,
		entity_type: entityType
	}));

	return {
		source,
		feed,
		entityType,
		sort,
		pageSize: PAGE_SIZE,
		entityTypes,
		itemSorts
	};
};

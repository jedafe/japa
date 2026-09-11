import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { Source, SourceItemEnvelope } from '$lib/types/sources';

const PAGE_SIZE = 24;
const VALID_ENTITY_TYPES = ['', 'scholarship', 'job', 'remittance', 'news', 'culture_lifestyle', 'guide', 'blog', 'article', 'wiki_page'];
const VALID_SORTS = ['latest', 'popular', 'discussed', 'upvoted'];

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;
	if (!slug) {
		error(404, { code: 'not_found', message: 'Source not found' });
	}

	const source = await apiGet<Source>(ctx, `/sources/${encodeURIComponent(slug)}`).catch(() => {
		error(404, { code: 'not_found', message: 'Source not found' });
	});

	const rawEntityType = event.url.searchParams.get('entity_type') ?? '';
	const entityType = VALID_ENTITY_TYPES.includes(rawEntityType) ? rawEntityType : '';

	const rawSort = event.url.searchParams.get('sort') ?? 'latest';
	const sort = VALID_SORTS.includes(rawSort) ? rawSort : 'latest';

	const params = new URLSearchParams();
	if (entityType) params.set('entity_type', entityType);
	params.set('sort', sort);
	params.set('page_size', String(PAGE_SIZE));
	params.set('page_offset', '0');

	const feed = await apiGet<SourceItemEnvelope>(ctx, `/sources/${encodeURIComponent(slug)}/items?${params.toString()}`).catch(() => ({
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
		pageSize: PAGE_SIZE
	};
};

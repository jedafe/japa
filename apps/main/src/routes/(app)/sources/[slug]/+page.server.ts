import { error, redirect } from '@sveltejs/kit';
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

	const sourceMetaPromise = apiGet<SourceMeta>(ctx, '/sources/meta').catch(() => ({
		categories: [],
		source_types: [],
		entity_types: [],
		sorts: [],
		item_sorts: []
	}));

	const source = await apiGet<Source>(ctx, `/sources/${encodeURIComponent(slug)}`).catch(() => {
		error(404, { code: 'not_found', message: 'Source not found' });
	});

	// §106: the backend also resolves a source by its UUID — canonicalize to
	// the slug URL so a channel never answers at two different addresses.
	if (source.slug && source.slug !== slug) {
		throw redirect(301, `/sources/${source.slug}${event.url.search}`);
	}

	const sourceMeta = await sourceMetaPromise;

	// Scope the filter pills to the entity types this source actually
	// publishes (GET /sources/{slug}.entity_types). The field is omitempty
	// on the backend — absent means "publishes nothing yet", so pills are
	// omitted entirely rather than offering guaranteed-empty feeds.
	const sourceTypes = source.entity_types ?? [];
	const entityTypes = (sourceMeta.entity_types ?? []).filter((e) =>
		sourceTypes.includes(e.value)
	);
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

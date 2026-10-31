import { error } from '@sveltejs/kit';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { PageServerLoad } from './$types';
import type { EntityWikiPage } from '../+page.server';
import type { WikiRevision } from '$lib/types';

// Revision history for an entity-bound wiki page —
// /wiki/e/{type_slug}/{entity}/{page_slug}/history. Lists field/content
// revisions targeting the bound entity (GET /wiki/entities/:type/:id/
// revisions), the entity counterpart of /wiki/{space}/{slug}/history.
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const { type_slug, entity, page_slug } = event.params;
	const page = Math.max(1, Number(event.url.searchParams.get('page') ?? '1') || 1);
	const limit = 25;
	try {
		const pageData = await apiGet<EntityWikiPage>(
			ctx,
			`/wiki/e/${encodeURIComponent(type_slug)}/${encodeURIComponent(entity)}/${encodeURIComponent(page_slug)}`
		);
		if (!pageData.kind || !pageData.entity_id) {
			error(404, { code: 'wiki_not_found', message: 'Entity wiki page not found' });
		}
		const revRes = await apiList<WikiRevision>(
			ctx,
			`/wiki/entities/${encodeURIComponent(pageData.kind)}/${encodeURIComponent(pageData.entity_id)}/revisions?page=${page}&limit=${limit}`
		).catch(() => null);
		return {
			page: pageData,
			revisions: revRes?.items ?? ([] as WikiRevision[]),
			meta: revRes?.meta ?? { total: revRes?.items.length ?? 0, page, limit }
		};
	} catch (e) {
		if (e instanceof ApiError) error(e.status, { code: e.code, message: e.message });
		throw e;
	}
};

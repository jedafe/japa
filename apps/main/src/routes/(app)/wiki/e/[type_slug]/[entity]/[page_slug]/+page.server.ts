import { error } from '@sveltejs/kit';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { PageServerLoad } from './$types';

export interface EntityWikiPage {
	id: string;
	kind: string;
	type_slug: string;
	entity_id: string;
	entity_slug: string;
	entity_name: string;
	slug: string;
	title: string;
	category: string;
	is_locked: boolean;
	is_featured: boolean;
	lazy: boolean;
	content?: unknown;
	field_group_id?: string;
	field_group_slug?: string;
	field_group_name?: string;
	fields?: {
		id: string; slug: string; name: string;
		fields: { key: string; label: string; field_type: string; value?: unknown; section?: string }[];
	};
}

// Entity-bound wiki page: /wiki/e/{type_slug}/{entity}/{page_slug}
// (JCE ↔ wiki mapping — visa routes, banks, embassies, courses, scholarships).
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const { type_slug, entity, page_slug } = event.params;
	try {
		const page = await apiGet<EntityWikiPage>(ctx, `/wiki/e/${type_slug}/${entity}/${page_slug}`);
		return { page };
	} catch (err) {
		if (err instanceof ApiError) {
			error(err.status === 404 ? 404 : 500, { code: 'wiki_page_error', message: err.message });
		}
		error(500, { code: 'wiki_page_error', message: 'Failed to load wiki page' });
	}
};

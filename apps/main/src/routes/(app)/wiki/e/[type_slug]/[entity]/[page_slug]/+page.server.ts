import { error, redirect } from '@sveltejs/kit';
import { apiContext, apiGet, ApiError } from '#lib/server/api.js';
import type { WikiHelpful } from '#lib/types/index.js';
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
	/** §43 — false on lazy/thin pages (no approved prose at the min-words
	 *  floor); the page emits noindex,follow. */
	indexable?: boolean;
	/** §wiki-seo — the type key the entity's pages canonicalize to (the
	 *  discriminator-matching type, e.g. 'student_visa' over generic
	 *  'visa_route'); absent when the kind resolves to a single type. */
	canonical_type_slug?: string;
	content?: unknown;
	field_group_id?: string;
	field_group_slug?: string;
	field_group_name?: string;
	fields?: {
		id: string; slug: string; name: string;
		fields: { key: string; label: string; field_type: string; value?: unknown; section?: string }[];
	};
	/** §wiki-links — category tab strip (lazy-resolvable type categories). */
	categories?: { slug: string; category: string; label: string }[];
	/** §wiki-links — mode:"related" peer listing (universities). */
	related?: {
		kind: string;
		items: { slug: string; name: string; geo_code?: string; offering_slug?: string }[];
		total: number;
	};
	/** Host-space breadcrumb chain (the entity's country/state/city) —
	 *  same shape as the space-page ancestors. */
	ancestors?: { slug: string; name: string; level?: string }[];
	ancestors_alt?: { slug: string; name: string; level?: string }[];
	siblings?: { slug: string; title: string; category?: string; current?: boolean }[];
	helpful?: WikiHelpful | null;
	revision_summary?: string;
	submitted_by?: string;
}

// Entity-bound wiki page: /wiki/e/{type_slug}/{entity}/{page_slug}
// (JCE ↔ wiki mapping — visa routes, banks, embassies, courses, scholarships).
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const { type_slug, entity, page_slug } = event.params;
	const authed = !!event.locals.user;
	const userPermissions = event.locals.user?.permissions ?? [];
	try {
		const page = await apiGet<EntityWikiPage>(ctx, `/wiki/e/${type_slug}/${entity}/${page_slug}`);
		// §43 crawl gate — lazy/thin entity pages carry noindex,follow.
		if (page.indexable !== true) {
			event.setHeaders({ 'x-robots-tag': 'noindex, follow' });
		}
		// §wiki-seo guest surface — entity stubs redirect guests to the
		// host-space index (real content or coming-soon) instead of
		// erroring; members see the stub as the contribution target.
		if (!authed && page.indexable !== true) {
			const host = page.ancestors?.[page.ancestors.length - 1]?.slug;
			redirect(303, host ? `/wiki/${encodeURIComponent(host)}` : '/wiki');
		}
		return { page, authed, userPermissions };
	} catch (err) {
		// redirect() throws — let it (and any non-API error) propagate
		// instead of collapsing into a 500.
		if (!(err instanceof ApiError)) throw err;
		error(err.status === 404 ? 404 : 500, { code: 'wiki_page_error', message: err.message });
	}
};

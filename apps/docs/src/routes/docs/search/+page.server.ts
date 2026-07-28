import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import { buildMeta, DOCS_ORIGIN } from '$lib/seo/meta';
import type { DocSearchResult } from '$lib/types';

/**
 * Server-side search results page (the ⌘K palette is the in-app search; this
 * page is the SEO-indexable destination reachable via the SearchAction JSON-LD).
 * Queries /docs/search?q=&limit= and renders grouped results.
 */
export const load: PageServerLoad = async (event) => {
	const q = (event.url.searchParams.get('q') ?? '').trim();
	let results: DocSearchResult[] = [];
	if (q.length >= 2) {
		try {
			results = await apiGet<DocSearchResult[]>(
				apiContext(event),
				`/docs/search?q=${encodeURIComponent(q)}&limit=20`
			);
		} catch {
			results = [];
		}
	}

	const meta = buildMeta({
		title: q ? `${t_results_for(q)} · JapaConnect Docs` : 'Search · JapaConnect Docs',
		description: 'Search the JapaConnect documentation.',
		path: q ? `/docs/search?q=${encodeURIComponent(q)}` : '/docs/search'
	});

	return { q, results, meta };
};

function t_results_for(q: string): string {
	return `Results for “${q}”`;
}

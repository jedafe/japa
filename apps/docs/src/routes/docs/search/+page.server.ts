import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import { buildMeta } from '$lib/seo/meta';
import type { DocSearchResult, DocAudience } from '$lib/types';
import { ValidAudiences } from '$lib/types';

/**
 * Server-side search results page (the ⌘K palette is the in-app search; this
 * page is the SEO-indexable destination reachable via the SearchAction JSON-LD).
 * Queries /docs/search?q=&audience=&limit= (ADR-060 — audience-filtered search).
 */
export const load: PageServerLoad = async (event) => {
	const q = (event.url.searchParams.get('q') ?? '').trim();
	const audience = (event.url.searchParams.get('audience') ?? '').trim() as DocAudience;

	let results: DocSearchResult[] = [];
	if (q.length >= 2) {
		try {
			const params = new URLSearchParams({ q, limit: '20' });
			if (audience && ValidAudiences.has(audience)) params.set('audience', audience);
			results = await apiGet<DocSearchResult[]>(apiContext(event), `/docs/search?${params}`);
		} catch {
			results = [];
		}
	}

	const meta = buildMeta({
		title: q ? `${resultsFor(q)} · JapaConnect Docs` : 'Search · JapaConnect Docs',
		description: 'Search the JapaConnect documentation.',
		path: q ? `/docs/search?q=${encodeURIComponent(q)}` : '/docs/search'
	});

	return { q, audience: audience || null, results, meta };
};

function resultsFor(q: string): string {
	return `Results for “${q}”`;
}

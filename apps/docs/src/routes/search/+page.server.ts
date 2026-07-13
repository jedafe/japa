import type { PageServerLoad } from './$types';
import { searchDocs } from '$lib/server/api';

export const load: PageServerLoad = async ({ url, locals }) => {
	const q = url.searchParams.get('q') ?? '';
	const token = (locals as any).token ?? null;

	if (q.length < 2) {
		return { query: q, results: [] as Awaited<ReturnType<typeof searchDocs>> };
	}

	try {
		const results = await searchDocs(q, token ?? undefined);
		return { query: q, results };
	} catch {
		return { query: q, results: [] as Awaited<ReturnType<typeof searchDocs>> };
	}
};

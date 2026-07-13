import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

type AutocompleteResult = {
	type: string;
	id: string;
	title: string;
};

export const load: PageServerLoad = async ({ url }) => {
	const q = url.searchParams.get('q');

	if (!q) {
		return { query: '', results: [] };
	}

	try {
		const result = await apiFetch<AutocompleteResult[]>(`/search/autocomplete?q=${encodeURIComponent(q)}`);
		if (result.ok && Array.isArray(result.data)) {
			return { query: q, results: result.data };
		}
		return { query: q, results: [] };
	} catch {
		return { query: q, results: [] };
	}
};

import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';

/**
 * Build a client-side Autocomplete fetcher over a fixed option list.
 * Matches on label (and id) case-insensitively; an empty query returns
 * the full list so it also works as `initialFetcher`.
 */
export function staticAutocompleteFetcher(
	items: AutocompleteItem[]
): (q?: string) => Promise<AutocompleteItem[]> {
	return async (q = '') => {
		const query = q.trim().toLowerCase();
		if (!query) return items;
		return items.filter(
			(i) => i.label.toLowerCase().includes(query) || i.id.toLowerCase().includes(query)
		);
	};
}

/**
 * Distinct {id,label} items derived from loaded list values (sorted,
 * deduped, blanks removed). Use for free-text filters where the backend
 * matches arbitrary strings — the observed corpus makes good suggestions.
 */
export function autocompleteItems(values: (string | null | undefined)[]): AutocompleteItem[] {
	return [...new Set(values.map((v) => v?.trim()).filter((v): v is string => !!v))]
		.sort((a, b) => a.localeCompare(b))
		.map((v) => ({ id: v, label: v }));
}

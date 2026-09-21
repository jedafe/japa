// Ref: docs/superpowers/plans/2026-09-11-sources-directory-v2-plan.md
// Public source proposal form. Loads the source metadata catalog from the
// backend so the form can render categories/source-kind options without
// hardcoding them (Rule 16).
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { SourceMeta } from '$lib/types/sources';

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	let meta: SourceMeta = { categories: [], source_types: [] };
	try {
		meta = await apiGet<SourceMeta>(ctx, '/sources/meta');
	} catch {
		// Metadata is optional for the form; fall back to the static v2 types.
	}
	return { meta };
};

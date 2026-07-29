// Ref: japaconnect-engineering/sections/16-page-builder.md (public page list)
// Lists all published Page Builder pages (GET /pages — public, no auth).
// Each card links to /pages/{slug} for the full rendered page.
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { PageBuilderPage } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	let pages: PageBuilderPage[] = [];
	try {
		const res = await apiList<PageBuilderPage>(ctx, '/pages');
		pages = res.items;
	} catch {
		pages = [];
	}
	return { pages };
};

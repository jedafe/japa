// Ref: docs/superpowers/plans/2026-09-11-sources-directory-v2-plan.md
// Public source proposal form. Loads the source metadata catalog from the
// backend so the form can render categories/source-kind options without
// hardcoding them (Rule 16).
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { SourceMeta } from '$lib/types/sources';

export const load: PageServerLoad = async (event) => {
	// Proposals require auth — without this gate guests could fill the whole
	// form only to hit raw 401s from /sources/probe and /sources/propose.
	if (!event.locals.user) {
		throw redirect(307, '/login?redirect=/sources/submit');
	}

	const ctx = apiContext(event);
	let meta: SourceMeta = { categories: [], source_types: [] };
	try {
		meta = await apiGet<SourceMeta>(ctx, '/sources/meta');
	} catch {
		// Metadata is optional for the form; fall back to the static v2 types.
	}
	return { meta };
};

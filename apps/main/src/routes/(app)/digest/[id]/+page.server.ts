// Ref: japaconnect-engineering/sections/84-community-digest-widget.md
// Single digest edition at /digest/[id] — SSR fetches GET /digest/{id}.
// Returns the edition or throws a 404 when not found.
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { DigestEdition } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const id = event.params.id;
	let digest: DigestEdition | null = null;
	let loadError = false;
	try {
		digest = await apiGet<DigestEdition>(ctx, `/digest/${id}`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Digest edition not found' });
		}
		digest = null;
		loadError = true;
	}
	return { digest, loadError };
};

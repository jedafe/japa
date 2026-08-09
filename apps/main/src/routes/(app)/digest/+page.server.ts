// Ref: japaconnect-engineering/sections/84-community-digest-widget.md
// Latest digest at /digest — SSR fetches GET /digest/latest (public).
// Returns the latest published edition or a loadError flag when none exists.
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { DigestEdition } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	let digest: DigestEdition | null = null;
	let loadError = false;
	try {
		digest = await apiGet<DigestEdition>(ctx, '/digest/latest');
	} catch {
		digest = null;
		loadError = true;
	}
	return { digest, loadError };
};

// Ref: japaconnect-engineering/sections/84-community-digest-widget.md
// Latest digest at /digest — SSR fetches GET /digest/latest (public).
// Returns the latest published edition or a loadError flag when none exists.
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { DigestEdition } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['digest.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Digest is not available' });
	}

	const ctx = apiContext(event);
	let digest: DigestEdition | null = null;
	let loadError = false;
	try {
		digest = await apiGet<DigestEdition>(ctx, '/digest/latest');
	} catch (err) {
		// 404 no_digest means simply "no published edition yet" — render the
		// empty state, not the error state. Other failures stay errors.
		if (err instanceof ApiError && err.status === 404) {
			digest = null;
		} else {
			digest = null;
			loadError = true;
		}
	}
	return { digest, loadError };
};

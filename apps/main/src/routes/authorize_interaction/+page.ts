import type { PageLoad } from './$types';

/**
 * OStatus remote-follow landing page at /authorize_interaction
 * (Section 126). Remote instances bounce followers here via the
 * WebFinger subscribe template ({site}/authorize_interaction?uri={uri});
 * the visitor enters their own fediverse handle and is redirected to
 * their home instance's authorize_interaction endpoint with our actor
 * as the target. Client-side only — no API call needed.
 *
 * Ref: japaconnect-engineering/sections/126-activitypub-federation.md
 */
export const load: PageLoad = async ({ url }) => {
	const uri = url.searchParams.get('uri') ?? '';
	return { uri };
};

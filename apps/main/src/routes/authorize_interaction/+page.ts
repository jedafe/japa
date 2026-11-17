import type { PageLoad } from './$types';

/**
 * OStatus remote-follow landing page at /authorize_interaction
 * (Section 126). Remote instances bounce followers here via the
 * WebFinger subscribe template ({site}/authorize_interaction?uri={uri});
 * the visitor enters their own fediverse handle and is redirected to
 * their home instance's authorize_interaction endpoint with our actor
 * as the target. Client-side only — no API call needed.
 *
 * The uri param is shape-checked to acct:/https:// actor identifiers —
 * the visitor's own instance does the real resolution, but this page
 * refuses to forward malformed or non-web schemes downstream. Plain
 * http:// actor IRIs are refused too: ActivityPub is https on the wire.
 *
 * Ref: japaconnect-engineering/sections/126-activitypub-federation.md
 */
export const load: PageLoad = async ({ url }) => {
	const raw = (url.searchParams.get('uri') ?? '').trim();
	const uri = /^(acct:|https:\/\/)/i.test(raw) ? raw : '';
	return { uri };
};

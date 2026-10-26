// Ref: japaconnect-engineering/sections/102-mentorship-matching.md
// Legacy redirect (services/mentorship unification): mentor discovery moved
// to the service directory's mentorship kind (/services?kind=mentorship).
// The mentorship.enabled gate is kept so a dark-launched flag still 404s
// exactly like the old page instead of landing on the bare directory.
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['mentorship.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Mentorship is not available' });
	}

	// Preserve the legacy filters that map onto the services directory
	// (dest, areas, include_unavailable, sort, page) — the mentorship-kind
	// branch of /services honours all of them (§102 discovery contract).
	const target = new URLSearchParams({ kind: 'mentorship' });
	for (const key of ['dest', 'areas', 'include_unavailable', 'sort', 'page'] as const) {
		const value = event.url.searchParams.get(key)?.trim();
		if (value) target.set(key, value);
	}
	throw redirect(308, `/services?${target.toString()}`);
};

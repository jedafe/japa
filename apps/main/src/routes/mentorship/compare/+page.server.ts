// Ref: japaconnect-engineering/adr/ADR-110-mentorship-matching.md
// Legacy redirect: /mentorship/compare → /compare/mentors (308 permanent),
// preserving the ?ids= parameter so old deep links keep working.
// /compare/mentors applies the mentorship.comparison flag gate and
// forwards to /compare?type=mentors&preselect=<ids>.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async (event) => {
	const ids = event.url.searchParams.get('ids') ?? '';
	const target = ids ? `/compare/mentors?ids=${encodeURIComponent(ids)}` : '/compare/mentors';
	throw redirect(308, target);
};

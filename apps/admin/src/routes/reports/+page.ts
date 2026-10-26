// /reports was merged into /moderation/reports — the canonical report queue
// with entity context and the full action set. Redirect deep links.
import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

export const load: PageLoad = () => {
	throw redirect(307, '/moderation/reports');
};

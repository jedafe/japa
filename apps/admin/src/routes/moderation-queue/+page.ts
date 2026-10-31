// /moderation-queue was merged into /moderation — the canonical review queue
// (same backend endpoint, full action set). The old alerts tab lives at
// /moderation/alerts. Redirect deep links, preserving filter/pagination
// params (status, content_type, page, q, sort, dir use the same names).
import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

export const load: PageLoad = ({ url }) => {
	const params = new URLSearchParams(url.searchParams);
	const tab = params.get('tab');
	params.delete('tab');
	const qs = params.toString();
	const target = tab === 'alerts' ? '/moderation/alerts' : '/moderation';
	throw redirect(307, qs ? `${target}?${qs}` : target);
};

import { redirect } from '@sveltejs/kit';

import type { RequestHandler } from './$types';

// Legacy RSS URL — the canonical thread feed lives at /thread/feed.xml
// (the URL the layout's autodiscovery link advertises); don't serve the
// same feed under a second indexable path.
export const GET: RequestHandler = () => {
	throw redirect(301, '/thread/feed.xml');
};

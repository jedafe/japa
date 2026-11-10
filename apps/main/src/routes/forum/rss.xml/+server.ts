import { redirect } from '@sveltejs/kit';

import type { RequestHandler } from './$types';

// Legacy RSS URL — the forum feed moved to /thread/feed.xml.
export const GET: RequestHandler = () => {
	throw redirect(301, '/thread/feed.xml');
};

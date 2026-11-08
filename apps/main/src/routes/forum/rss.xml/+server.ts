import { redirect } from '@sveltejs/kit';

import type { RequestHandler } from './$types';

// Legacy RSS URL — the forum feed moved to /thread/rss.xml.
export const GET: RequestHandler = () => {
	throw redirect(301, '/thread/rss.xml');
};

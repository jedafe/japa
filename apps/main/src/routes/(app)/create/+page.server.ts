// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md §10 — the composer moved
// to /studio/create; /create is kept as a redirect so existing links, draft
// restore URLs (?draft=<id>), and login redirect targets keep working.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async (event) => {
	throw redirect(308, `/studio/create${event.url.search}`);
};

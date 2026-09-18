// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md — /studio is a tabbed
// surface; the index route lands on the Videos tab to preserve pre-tabs behavior.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async () => {
	throw redirect(307, '/studio/videos');
};

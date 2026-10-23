// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md — /studio is a tabbed
// surface; the index lands on Contents, the first tab and the only one that is
// never feature-flag gated (Videos 307s to /studio/create when video.enabled
// is off, so landing there produced a double redirect and a dead end).
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async () => {
	throw redirect(307, '/studio/contents');
};

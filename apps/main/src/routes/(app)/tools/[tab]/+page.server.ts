import { redirect, error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { TOOL_SECTIONS, type ToolSection } from '$lib/tools/registry';

/**
 * /tools/:tab — shareable hub section permalink (§78 Tools Web Surface).
 *
 * The hub uses ?tab=... client-side. This route validates the section slug and
 * redirects to the canonical hub with the same tab query parameter so bookmarks
 * and shared links work without duplicating the hub UI.
 */
export const load: PageServerLoad = async (event) => {
	const tab = event.params.tab as string;
	if (!TOOL_SECTIONS.includes(tab as ToolSection)) {
		throw error(404, { code: 'tab_not_found', message: 'Tool section not found' });
	}
	throw redirect(302, `/tools?tab=${encodeURIComponent(tab)}`);
};

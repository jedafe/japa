import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import { mainAppUrl } from '$lib/seo';

/**
 * JapaBot standalone — §20, §69.
 *
 * The canonical JapaBot chat UI lives on the main app's unified /ai
 * surface, chat mode (single implementation: conversation sidebar, RAG
 * citations, model/voice pickers). This page is now a landing surface
 * only: guests see the tool's SEO + sign-in prompt; signed-in users are
 * redirected to /ai?mode=chat so there is exactly one chat experience.
 *
 * Ref: §20 (AI Core), §69 (RAG Knowledge Layer), ROADMAP-TOOLS 02.07.
 */
export const load: PageServerLoad = async (event) => {
	// Gate: skip all backend calls when ai.japabot_enabled is false.
	// Ref: conventions/system-settings-catalog.md, Rule 3.
	const tool = getTool('japabot')!;
	if (!isToolEnabled(tool, event.locals.features)) {
		return { enabled: false, tool, user: event.locals.user };
	}

	// Signed-in users go straight to the canonical chat surface.
	if (event.locals.user) {
		throw redirect(302, mainAppUrl('/ai?mode=chat'));
	}

	return { enabled: true, tool, user: null };
};

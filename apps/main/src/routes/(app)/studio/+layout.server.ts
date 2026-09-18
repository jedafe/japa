// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md — /studio tabbed surface.
// Auth + feature flags for the tab strip. The video.enabled gate itself lives
// in videos/+page.server.ts; here it only controls the disabled tab state.
import { redirect } from '@sveltejs/kit';
import type { LayoutServerLoad } from './$types';

export const load: LayoutServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(
			307,
			`/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`
		);
	}

	const parent = await event.parent();
	const siteSettings = (parent as { siteSettings?: Record<string, unknown> }).siteSettings ?? {};

	return {
		videoEnabled: siteSettings['video.enabled'] === true
	};
};

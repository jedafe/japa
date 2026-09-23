// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md — /studio tabbed surface.
// Ref: .jeos/specs/2026-09-23-studio-moderation-tab.md — Moderation tab gate.
// Auth + feature flags for the tab strip. The video.enabled gate itself lives
// in videos/+page.server.ts; here it only controls the disabled tab state.
import { redirect } from '@sveltejs/kit';
import type { LayoutServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';

interface ModerationScope {
	platform_moderator: boolean;
	moderated_spaces: { id: string; name: string; slug: string }[];
	moderated_tags: { id: string; name: string; slug: string }[];
	assigned_count: number;
	open_count: number;
}

export const load: LayoutServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(
			307,
			`/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`
		);
	}

	const parent = await event.parent();
	const siteSettings = (parent as { siteSettings?: Record<string, unknown> }).siteSettings ?? {};

	// Moderation tab is visible when the caller has any moderation scope.
	// Degrade silently to hidden when the scope endpoint is unreachable.
	let moderationScope: ModerationScope | null = null;
	try {
		moderationScope = await apiGet<ModerationScope>(apiContext(event), '/me/moderation/scope');
	} catch {
		moderationScope = null;
	}

	return {
		videoEnabled: siteSettings['video.enabled'] === true,
		moderationScope
	};
};

// Ref: japaconnect-engineering/sections/79-community-story-wall.md
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/**
 * Story submission moved into the unified composer at /create?type=story.
 * Keep this route as a redirect so existing links/bookmarks continue to
 * work. Auth-gated: guests are sent to login with a return redirect to
 * the composer story form.
 */
export const load: PageServerLoad = async (event) => {
	const target = '/create?type=story';
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(target)}`);
	}
	throw redirect(307, target);
};

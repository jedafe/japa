// Ref: japaconnect-engineering/sections/80-destination-wiki.md
import type { PageServerLoad } from './$types';

/**
 * Wiki page creation at /wiki/{space_slug}/create.
 * Returns auth/permission flags so the UI can gate the form.
 */
export const load: PageServerLoad = async (event) => {
	const spaceSlug = event.params.space_slug;
	const authed = !!event.locals.user;
	const userPermissions = event.locals.user?.permissions ?? [];
	return { spaceSlug, authed, userPermissions };
};

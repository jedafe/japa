// Ref: japaconnect-engineering/sections/50-current-user.md (bookmarks)
// Lists the user's bookmarks and bookmark folders. Auth-gated; the backend
// returns thin records (entity_type + entity_id + folder + created_at) — the
// page links to the entity by deriving the URL from the entity_type.
// Folders are fetched from GET /me/bookmark-folders and managed client-side
// via POST/PATCH/DELETE /me/bookmark-folders.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { Bookmark, BookmarkFolder } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}
	const ctx = apiContext(event);
	const [bookmarksRes, foldersRes] = await Promise.all([
		apiList<Bookmark>(ctx, '/me/bookmarks'),
		apiList<BookmarkFolder>(ctx, '/me/bookmark-folders')
	]);
	return { bookmarks: bookmarksRes.items, folders: foldersRes.items };
};

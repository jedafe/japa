// Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md (bookmarks)
// Lists the user's bookmarks and bookmark folders. Auth-gated; the backend
// returns thin records (entity_type + entity_id + folder + folder_id +
// created_at) — the page links to the entity by deriving the URL from
// the entity_type.
// Folders are fetched from GET /me/bookmark-folders and managed client-side
// via POST/PATCH/DELETE /me/bookmark-folders.
// When ?folder_id=uuid is present, the backend filters bookmarks server-side.
// ?type=<entity_type> is forwarded as the backend's entity_type filter.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { Bookmark, BookmarkFolder } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}
	const ctx = apiContext(event);
	const folderId = event.url.searchParams.get('folder');
	const entityType = event.url.searchParams.get('type')?.trim() ?? '';
	// page_size=100 (endpoint max) — the default is 20 and the response has no
	// meta total, so the client pages further via page_offset.
	const params = new URLSearchParams({ page_size: '100' });
	if (folderId) params.set('folder_id', folderId);
	if (entityType) params.set('entity_type', entityType);
	const [bookmarksRes, foldersRes] = await Promise.all([
		apiList<Bookmark>(ctx, `/me/bookmarks?${params.toString()}`),
		apiList<BookmarkFolder>(ctx, '/me/bookmark-folders')
	]);
	return {
		bookmarks: bookmarksRes.items,
		folders: foldersRes.items,
		activeFolderId: folderId,
		activeType: entityType
	};
};

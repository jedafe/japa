// Ref: japaconnect-engineering/sections/34-media.md (user media library)
// Lists the user's uploaded media. Auth-gated; the backend returns a
// paginated envelope { data, total, total_pages } with full MediaResponse
// rows (url, filename, mime_type, file_size, alt_text, status). The client
// component drives filter/search/pagination via /me/media directly.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGetEnvelope } from '$lib/server/api';
import type { MediaAsset } from '$lib/types';

const PAGE_SIZE = 24;

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}
	const ctx = apiContext(event);
	const params = new URLSearchParams();
	params.set('page_size', String(PAGE_SIZE));
	params.set('page_offset', '0');
	// /me/media returns { data: MediaAsset[], total, total_pages }.
	// apiGetEnvelope returns the full envelope so we can read total/total_pages.
	const envelope = await apiGetEnvelope<MediaAsset[]>(ctx, `/me/media?${params.toString()}`);
	const items = envelope.data ?? [];
	const total = (envelope as unknown as { total?: number }).total ?? items.length;
	const total_pages = (envelope as unknown as { total_pages?: number }).total_pages ?? 1;
	return { media: { items, total, total_pages } };
};

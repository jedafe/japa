// Ref: japaconnect-engineering/sections/34-media.md (user media library)
// Lists the user's uploaded media. Auth-gated; the backend returns full
// MediaResponse rows (url, filename, mime_type, file_size, alt_text, status).
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { MediaAsset } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}
	const ctx = apiContext(event);
	const media = (await apiList<MediaAsset>(ctx, '/me/media')).items;
	return { media };
};

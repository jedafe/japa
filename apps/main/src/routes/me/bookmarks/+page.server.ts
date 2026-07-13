import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

type Bookmark = {
	id: string;
	content_id: string;
	title: string;
	content_type: string;
	folder_id?: string;
	created_at: string;
};

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('jc_token');

	try {
		const result = await apiFetch<Bookmark[]>('/me/bookmarks', {}, token);
		if (result.ok) {
			return { data: result.data };
		}
		return { data: null, error: result.error };
	} catch {
		return { data: null, error: 'Failed to load' };
	}
};
